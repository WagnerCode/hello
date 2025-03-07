package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"

	"go.mongodb.org/mongo-driver/mongo/options"
)

const mongoURI = "mongodb://localhost:27017"
const databaseName = "fileStore"

func main() {
	// Connect to MongoDB
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		fmt.Println("Error connecting to MongoDB:", err)
		return
	}

	// Check connection
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

	// Open the download stream
	downloadStream, err := gridFSBucket.OpenDownloadStreamByName("2025-03-05_09:11:513.pdf")
	if err != nil {
		fmt.Println("Error opening download stream:", err)
		return
	}
	defer downloadStream.Close()

	// Create a file to save the downloaded data
	outputFile, err := os.Create("downloaded_image.pdf")
	if err != nil {
		fmt.Println("Error creating output file:", err)
		return
	}
	defer outputFile.Close()

	// Copy the data from the download stream to the output file
	_, err = io.Copy(outputFile, downloadStream)
	if err != nil {
		fmt.Println("Error downloading data:", err)
		return
	}

	fmt.Println("Image downloaded successfully as downloaded_image.png")
}
