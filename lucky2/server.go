package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
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

const mongoURI = "mongodb://localhost:27017"
const databaseName = "fileStore"

func main() {
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
	fmt.Println("Server listening on port", port)

	client := connectToDB()
	defer client.Disconnect(context.TODO())

	gridFSBucket, err := gridfs.NewBucket(client.Database(databaseName))
	if err != nil {
		fmt.Println("Error creating GridFS bucket:", err)
		return
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		<-signalChan
		listener.Close()
		client.Disconnect(context.TODO())
		os.Exit(0)
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		fmt.Println("Got connection from", conn.RemoteAddr())

		go handleConnection(conn, gridFSBucket)
	}
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
