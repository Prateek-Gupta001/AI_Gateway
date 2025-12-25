package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func main() {
	resp, err := http.Get("http://localhost:8080/test-stream")
	fmt.Println("Got this resp ", resp)
	if err != nil {
		fmt.Println("Got this err ", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	var llmResponse string
	for {
		data, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break // stream ended
			}
			// real error
			fmt.Println("err ", err)
		}
		// line contains ONE streamed chunk
		if data != "\n" {
			data := strings.TrimPrefix(data, "data: ")
			fmt.Println("chunk:", data)
			llmResponse += data
			if data == "[DONE]" {
				fmt.Println("streaming done!")
			}
		}
	}
}
