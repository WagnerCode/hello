package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"

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

	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		fmt.Println("Error connecting to MongoDB:", err)
		return
	}
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		fmt.Println("Could not ping MongoDB:", err)
		return
	}
	fmt.Println("Connected to MongoDB!")

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
		fmt.Println("Got connection from", conn.RemoteAddr())

		var fileNameLen int32
		if err := binary.Read(conn, binary.BigEndian, &fileNameLen); err != nil {
			fmt.Println("Error reading filename length:", err)
			conn.Close()
			continue
		}
		fmt.Println("len of filename:", fileNameLen)

		fileName := make([]byte, fileNameLen)
		if _, err := io.ReadFull(conn, fileName); err != nil {
			fmt.Println("Error reading filename:", err)
			conn.Close()
			continue
		}

		fmt.Printf("Receiving data for file: %s\n", fileName)

		uploadStream, err := gridFSBucket.OpenUploadStream(string(fileName))
		if err != nil {
			fmt.Println("Error opening upload stream:", err)
			conn.Close()
			continue
		}

		_, err = io.Copy(uploadStream, conn)
		if err != nil {
			fmt.Println("Error uploading data:", err)
		} else {
			fmt.Println("Data received and uploaded successfully")
		}

		uploadStream.Close()
		conn.Close()
	}
}
