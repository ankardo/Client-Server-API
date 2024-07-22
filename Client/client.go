package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:8080/cotacao", nil)
	if err != nil {
		log.Printf("Error creating request: %v\n", err)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Error sending request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			log.Printf("Error reading response body: %v\n", readErr)
			return
		}
		log.Printf("Error response from server: %s\n", string(body))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v\n", err)
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		log.Printf("Error decoding JSON: %v\n", err)
		return
	}

	bid, ok := data["bid"].(float64)
	if !ok {
		log.Printf("Invalid response format: quote value not found or not a number\n")
		return
	}

	bidStr := strconv.FormatFloat(bid, 'f', 2, 64)

	content := []byte("Dólar:" + bidStr)
	if err := os.WriteFile("cotacao.txt", content, 0644); err != nil {
		log.Printf("Error writing to file: %v\n", err)
		return
	}

	fmt.Println("Dollar quotation saved successfully")
}
