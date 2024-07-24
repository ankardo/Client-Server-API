package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

const (
	requestURL = "https://economia.awesomeapi.com.br/json/last/USD-BRL"
	timeoutAPI = 200 * time.Millisecond
	timeoutDB  = 10 * time.Millisecond
)

type Quote struct {
	Bid        float64   `json:"bid"`
	Timestamp  int64     `json:"timestamp"`
	CreateDate time.Time `json:"create_date"`
}

type ClientResponse struct {
	Bid float64 `json:"bid"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/cotacao", getDollarQuotationHandler)
	http.ListenAndServe(":8080", mux)
}

func connectDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", "../dollarQuotation.db")
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}
	return db, nil
}

func ensureQuoteExists(db *sql.DB) error {
	createTableSQL := `
    CREATE TABLE IF NOT EXISTS quotes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        bid DECIMAL(10, 4) NOT NULL,
        timestamp BIGINT NOT NULL,
        create_date DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP)
    );`

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("error creating quotes table: %v", err)
	}

	return nil
}

func getDollarQuotation() (*Quote, error) {
	ctxAPI, cancelAPI := context.WithTimeout(context.Background(), timeoutAPI)
	defer cancelAPI()

	req, err := http.NewRequestWithContext(ctxAPI, "GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %v", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var data map[string]interface{}
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("error decoding JSON: %v", err)
	}

	rate := data["USDBRL"].(map[string]interface{})
	bid, err := strconv.ParseFloat(rate["bid"].(string), 64)
	if err != nil {
		return nil, fmt.Errorf("error parsing bid: %v", err)
	}

	timestampStr := rate["timestamp"].(string)
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %v", err)
	}

	createDateStr := rate["create_date"].(string)
	createDate, err := time.Parse("2006-01-02 15:04:05", createDateStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing create_date: %v", err)
	}

	quote := &Quote{
		Bid:        bid,
		Timestamp:  timestamp,
		CreateDate: createDate,
	}
	return quote, nil
}

func saveIfTimestampChanged(db *sql.DB, newQuote *Quote) error {
	ctxDB, cancelDB := context.WithTimeout(context.Background(), timeoutDB)
	defer cancelDB()

	var currentTimestamp int64
	err := db.QueryRowContext(ctxDB, "SELECT timestamp FROM quotes ORDER BY id DESC LIMIT 1").
		Scan(&currentTimestamp)
	switch {
	case err == sql.ErrNoRows:
		return insertQuote(ctxDB, db, newQuote)
	case err != nil:
		return fmt.Errorf("error querying database: %v", err)
	}

	if newQuote.Timestamp != currentTimestamp {
		return insertQuote(ctxDB, db, newQuote)
	}

	return nil
}

func insertQuote(ctx context.Context, db *sql.DB, quote *Quote) error {
	_, err := db.ExecContext(
		ctx,
		"INSERT INTO quotes (bid, timestamp, create_date) VALUES (?, ?, ?)",
		quote.Bid,
		quote.Timestamp,
		quote.CreateDate,
	)
	if err != nil {
		return fmt.Errorf("error inserting quote into database: %v", err)
	}
	fmt.Printf("Quote saved successfully\n")
	return nil
}

func getDollarQuotationHandler(w http.ResponseWriter, r *http.Request) {
	quote, err := getDollarQuotation()
	if err != nil {
		http.Error(
			w,
			fmt.Sprintf("Failed to fetch quotation: %v", err),
			http.StatusInternalServerError,
		)
		return
	}

	db, err := connectDB()
	if err != nil {
		http.Error(
			w,
			fmt.Sprintf("Failed to connect to database: %v", err),
			http.StatusInternalServerError,
		)
		return
	}
	defer db.Close()

	if err = ensureQuoteExists(db); err != nil {
		http.Error(
			w,
			fmt.Sprintf("Failed to create quotes table: %v", err),
			http.StatusInternalServerError,
		)
		return
	}

	if err = saveIfTimestampChanged(db, quote); err != nil {
		http.Error(
			w,
			fmt.Sprintf("Failed to save quotation: %v", err),
			http.StatusInternalServerError,
		)
		return
	}
	response := ClientResponse{
		Bid: quote.Bid,
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		http.Error(
			w,
			fmt.Sprintf("Failed to serialize quotation to JSON: %v", err),
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(responseJSON)
}
