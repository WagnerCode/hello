package main

import (
	"fmt"
	"io"
	"net"
	"os"
)

func main() {
	// Connect to the server
	serverAddress := "10.10.13.19:55000"
	conn, err := net.Dial("tcp", serverAddress)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}

	// Provide the file to send

	filePath, err := os.Getwd()
	if err != nil {
		fmt.Println(err) //print the error if obtained
	}

	// Step 1: Send filename first
	fileName := "frame9.png" // Change to the file's name you want to send
	_, err = conn.Write([]byte(fileName))
	if err != nil {
		fmt.Println("Error sending filename:", err)
		return
	}

	// Step 2: Open the file
	file, err := os.Open(filePath + "/" + fileName)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	// Step 3: Send the file data
	_, err = io.Copy(conn, file)
	if err != nil {
		fmt.Println("Error sending file data:", err)
		return
	}

	fmt.Println("File sent successfully!")
}
