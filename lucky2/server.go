package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func getLocalIP() (string, error) {
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipv4 := addr.To4(); ipv4 != nil {
			return ipv4.String(), nil
		}
	}
	return "", fmt.Errorf("no IPv4 address found")
}

func connectToDB() *mongo.Client {
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		fmt.Println("Error connecting to MongoDB:", err)
		return nil
	}
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		fmt.Println("Could not ping MongoDB:", err)
		return nil
	}
	fmt.Println("Connected to MongoDB!")
	return client
}

const httpPort = ":5000"
const mongoURI = "mongodb://localhost:27017"
const databaseName = "fileStore"

func main() {
	go func() {
		port := ":55000"
		localIP, err := getLocalIP()
		if err != nil {
			fmt.Println("Error getting local IP:", err)
			return
		}

		listener, err := net.Listen("tcp", localIP+port)
		if err != nil {
			fmt.Println("Error starting server:", err)
			return
		}
		defer listener.Close()
		fmt.Println("TCP Server listening on port", port)

		client := connectToDB()
		defer client.Disconnect(context.TODO())

		gridFSBucket, err := gridfs.NewBucket(client.Database(databaseName))
		if err != nil {
			fmt.Println("Error creating GridFS bucket:", err)
			return
		}

		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Println("Error accepting connection:", err)
				continue
			}
			go handleConnection(conn, gridFSBucket)
		}
	}()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.Handle("/download", authMiddleware(http.HandlerFunc(downloadHandler)))
	http.Handle("/files", authMiddleware(http.HandlerFunc(filesListHandler)))

	httpServer := &http.Server{Addr: httpPort}
	go func() {
		fmt.Printf("HTTP server started on %s\n", httpPort)
		if err := httpServer.ListenAndServe(); err != nil {
			fmt.Println("HTTP server error:", err)
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
	fmt.Println("Shutting down...")
}

func handleConnection(conn net.Conn, gridFSBucket *gridfs.Bucket) {
	defer conn.Close()
	var fileNameLen int32
	if err := binary.Read(conn, binary.BigEndian, &fileNameLen); err != nil {
		fmt.Println("Error reading filename length:", err)
		conn.Close()
		return
	}
	if fileNameLen <= 0 || fileNameLen > 255 {
		fmt.Println("Invalid filename length")
		conn.Close()
		return
	}
	fmt.Println("len of filename:", fileNameLen)

	fileName := make([]byte, fileNameLen)
	if _, err := io.ReadFull(conn, fileName); err != nil {
		fmt.Println("Error reading filename:", err)
		conn.Close()
		return
	}

	var clientIDLen int32
	if err := binary.Read(conn, binary.BigEndian, &clientIDLen); err != nil {
		fmt.Println("Error reading ClientID length:", err)
		return
	}

	clientID := make([]byte, clientIDLen)
	if _, err := io.ReadFull(conn, clientID); err != nil {
		fmt.Println("Error reading ClientID:", err)
		return
	}

	fmt.Printf("Receiving data for file: %s (ClientID: %s)\n", fileName, clientID)

	opts := options.GridFSUpload().
		SetMetadata(bson.D{{"clientID", string(clientID)}})

	uploadStream, err := gridFSBucket.OpenUploadStream(
		string(fileName),
		opts,
	)
	if err != nil {
		fmt.Println("Error opening upload stream:", err)
		conn.Close()
		return
	}

	_, err = io.Copy(uploadStream, conn)
	if err != nil {
		fmt.Println("Error uploading data:", err)
	} else {
		fmt.Println("Data received and uploaded successfully")
	}
	uploadStream.Close()
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		http.Error(w, "Filename is required", http.StatusBadRequest)
		return
	}

	// Подключаемся к MongoDB
	client := connectToDB()
	defer client.Disconnect(context.TODO())

	gridFSBucket, err := gridfs.NewBucket(client.Database(databaseName))
	if err != nil {
		http.Error(w, "Error connecting to GridFS", http.StatusInternalServerError)
		return
	}

	// Проверяем существование файла

	// Скачиваем файл
	downloadStream, err := gridFSBucket.OpenDownloadStreamByName(filename)
	if err != nil {
		http.Error(w, "Error opening file", http.StatusInternalServerError)
		return
	}
	defer downloadStream.Close()

	// Устанавливаем заголовки
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	// Отправляем данные
	if _, err := io.Copy(w, downloadStream); err != nil {
		http.Error(w, "Error sending file", http.StatusInternalServerError)
		return
	}
}

func filesListHandler(w http.ResponseWriter, r *http.Request) {
	// Подключаемся к MongoDB
	client := connectToDB()
	defer client.Disconnect(context.TODO())

	// Получаем коллекцию GridFS
	db := client.Database(databaseName)
	filesCollection := db.Collection("fs.files") // Коллекция с метаданными файлов

	// Выбираем все документы d
	cursor, err := filesCollection.Find(context.Background(), bson.M{})
	if err != nil {
		http.Error(w, "Error fetching files", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(context.Background())

	// Собираем имена файлов
	var filenames []string
	for cursor.Next(context.Background()) {
		var file struct {
			Filename string `bson:"filename"`
		}
		if err := cursor.Decode(&file); err != nil {
			http.Error(w, "Error decoding file", http.StatusInternalServerError)
			return
		}
		filenames = append(filenames, file.Filename)
	}

	// Отображаем шаблон с списком файлов
	tmpl := template.Must(template.New("files").Parse(`
	<!DOCTYPE html>
	<html lang="en">
	<head>
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<title>Файл менеджер</title>
		<style>
			body {
				font-family: 'Arial', sans-serif;
				background: #1a1a1a;
				color: #f5f5f5;
				margin: 0;
				padding: 0;
			}
	
			.container {
				max-width: 800px;
				margin: 2rem auto;
				padding: 2rem;
				background: #2d2d2d;
				border-radius: 10px;
				box-shadow: 0 0 30px rgba(0,0,0,0.15);
			}
	
			h2 {
				color: #3498db;
				border-bottom: 3px solid #27ae60;
				padding-bottom: 10px;
				margin-bottom: 2rem;
			}
	
			ul {
				list-style: none;
				padding: 0;
				margin: 0;
			}
	
			li {
				background: #333;
				margin: 10px 0;
				padding: 15px;
				border-radius: 5px;
				transition: all 0.3s ease;
			}
	
			li:hover {
				background: #444;
				transform: translateY(-2px);
				box-shadow: 0 5px 15px rgba(0,0,0,0.2);
			}
	
			a {
				color: #ffffff;
				text-decoration: none;
				font-weight: 500;
			}
	
			a:hover {
				color: #3498db;
			}
	
			.back-link {
				display: block;
				text-align: center;
				margin-top: 2rem;
				padding: 10px 20px;
				background: #27ae60;
				border-radius: 5px;
				transition: all 0.3s ease;
			}
	
			.back-link:hover {
				background: #2ecc71;
			}
		</style>
	</head>
	<body>
		<div class="container">
			<h2>Файл менеджер</h2>
			<ul>
				{{range .}}
					<li><a href="/download?filename={{.}}">{{.}}</a></li>
				{{end}}
			</ul>
			<a href="/logout" class="back-link">Logout</a>
		</div>
	</body>
	</html>
    `))
	tmpl.Execute(w, filenames)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		tmpl := template.Must(template.New("login").Parse(`
        <!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Авторизация</title>
    <style>
        /* Фоновая градиентная оболочка */
        body {
            margin: 0;
            min-height: 100vh;
            display: flex;
            justify-content: center;
            align-items: center;
            background: linear-gradient(45deg, #2c3e50, #4a6572);
            font-family: 'Arial', sans-serif;
        }

        /* Контейнер формы */
        .login-container {
            background: rgba(255, 255, 255, 0.15);
            border-radius: 15px;
            padding: 40px;
            backdrop-filter: blur(10px);
            box-shadow: 0 8px 32px 0 rgba(31, 38, 135, 0.37);
            width: 350px;
            text-align: center;
            transition: all 0.3s ease;
        }

        /* Заголовок */
        h2 {
            color: #ecf0f1;
            font-weight: 700;
            margin-bottom: 30px;
            position: relative;
        }

        h2::before {
            content: '';
            position: absolute;
            width: 100%;
            height: 5px;
            background: linear-gradient(90deg, #27ae60, #2ecc71);
            bottom: -10px;
            left: 0;
            border-radius: 5px;
        }

        /* Поле ввода */
        .input-field {
            width: 100%;
            padding: 12px;
            margin: 15px 0;
            background: rgba(255, 255, 255, 0.1);
            border: none;
            border-radius: 8px;
            color: #ecf0f1;
            font-size: 1em;
            transition: all 0.3s ease;
            outline: none;
        }

        .input-field:focus {
            box-shadow: 0 0 10px rgba(39, 174, 96, 0.5);
            background: rgba(255, 255, 255, 0.2);
        }

        /* Кнопка входа */
        .login-btn {
            width: 100%;
            padding: 12px;
            background: linear-gradient(90deg, #27ae60, #2ecc71);
            border: none;
            border-radius: 8px;
            color: #fff;
            font-size: 1em;
            cursor: pointer;
            transition: all 0.3s ease;
        }

        .login-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 5px 15px rgba(39, 174, 96, 0.5);
        }

        

        /* Стили для ошибок */
        .error {
            color: #e74c3c;
            margin-top: 10px;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <h2>Вход</h2>
        <form action="/login" method="POST">
            <input type="text" 
                   name="username" 
                   class="input-field" 
                   placeholder="Логин" 
                   required>
            <input type="password" 
                   name="password" 
                   class="input-field" 
                   placeholder="Пароль" 
                   required>
            <button type="submit" class="login-btn">Войти</button>
        </form>
    </div>
</body>
</html>
        `))
		tmpl.Execute(w, nil)
		return
	}
	if r.Method == "POST" {
		r.ParseForm()
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "root" && password == "root" {

			cookie := &http.Cookie{
				Name:     "auth",
				Value:    "trueWithSimpleDefence",
				Expires:  time.Now().Add(24 * time.Hour),
				Path:     "/",
				HttpOnly: true,
			}
			http.SetCookie(w, cookie)
			http.Redirect(w, r, "/files", http.StatusSeeOther)
			return
		}

		http.Error(w, "Неверный логин или пароль", http.StatusForbidden)
	}
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth")
		if err != nil || cookie.Value != "trueWithSimpleDefence" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "auth",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
