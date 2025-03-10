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

	http.HandleFunc("/download", downloadHandler)
	http.HandleFunc("/", homePage)
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

func homePage(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("home").Parse(`
	<html>
	<body>
	<h2>Скачать файл</h2>
	<form action="/download" method="GET">
	Имя файла в базе данных: <input type="text" name="filename" required>
	<input type="submit" value="Скачать">
	</form>
	</body>
	</html>
	`))
	tmpl.Execute(w, nil)
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
