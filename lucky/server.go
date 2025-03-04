package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"

	"go.mongodb.org/mongo-driver/mongo/options"
)

// getLocalIP retrieves the local IP address of the machine.
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

// DB connection settings
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

	// Connect to MongoDB
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		fmt.Println("Error connecting to MongoDB:", err)
		return
	}

	// Check ping
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		fmt.Println("Could not ping MongoDB:", err)
		return
	}
	fmt.Println("Connected to MongoDB!")

	// Initialize GridFS
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

		// Upload file to MongoDB using GridFS
		fmt.Println("Receiving data...")
		uploadStream, err := gridFSBucket.OpenUploadStream("received_frame1.png")
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
