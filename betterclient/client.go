package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
)

func main() {
	serverAddress := "10.10.13.19:55000"
	conn, err := net.Dial("tcp", serverAddress)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}
	defer conn.Close()

	fileName := "frame9.png"
	file, err := os.Open(fileName)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	// Sending the length of the filename
	fileNameLen := int32(len(fileName))
	if err := binary.Write(conn, binary.BigEndian, fileNameLen); err != nil {
		fmt.Println("Error sending filename length:", err)
		return
	}

	// Sending the filename
	if _, err := conn.Write([]byte(fileName)); err != nil {
		fmt.Println("Error sending filename:", err)
		return
	}

	// Sending the file content
	if _, err := io.Copy(conn, file); err != nil {
		fmt.Println("Error sending file data:", err)
		return
	}

	fmt.Println("File sent successfully!")
}
