package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"github.com/openmeet-team/survey/internal/db"
	"github.com/openmeet-team/survey/internal/telemetry"
)

// JetstreamClient manages the WebSocket connection to Jetstream
type JetstreamClient struct {
	url       string
	queries   *db.Queries
	processor *Processor
	conn      *websocket.Conn
	done      chan struct{}
}

// NewJetstreamClient creates a new Jetstream client
func NewJetstreamClient(url string, queries *db.Queries) *JetstreamClient {
	return &JetstreamClient{
		url:       url,
		queries:   queries,
		processor: NewProcessor(queries),
		done:      make(chan struct{}),
	}
}

// Connect establishes the WebSocket connection with cursor resumption
func (c *JetstreamClient) Connect(ctx context.Context) error {
	// Get current cursor
	cursor, err := GetCursor(ctx, c.queries)
	if err != nil {
		return fmt.Errorf("failed to get cursor: %w", err)
	}

	// Build URL with cursor if > 0
	url := c.url
	if cursor > 0 {
		url = fmt.Sprintf("%s&cursor=%d", c.url, cursor)
	}

	log.Printf("Connecting to Jetstream: %s", url)

	// Establish WebSocket connection
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to dial websocket: %w", err)
	}

	c.conn = conn
	telemetry.JetstreamConnectionStatus.Set(1)
	log.Printf("Connected to Jetstream (resuming from cursor: %d)", cursor)

	return nil
}

// Run starts the message processing loop
func (c *JetstreamClient) Run(ctx context.Context) error {
	defer close(c.done)

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down Jetstream client...")
			return nil
		default:
			// Read message from WebSocket
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("error reading message: %w", err)
			}

			// Parse the message
			var msg JetstreamMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("ERROR: Failed to unmarshal message: %v", err)
				continue
			}

			// Process the message with cursor update and metrics
			collection := ""
			operation := ""
			if msg.Commit != nil {
				collection = msg.Commit.Collection
				operation = msg.Commit.Operation
			}

			startTime := time.Now()
			if err := c.processor.ProcessMessageWithCursor(ctx, &msg, c.queries.GetDB); err != nil {
				log.Printf("ERROR: Failed to process message: %v", err)
				telemetry.JetstreamRecordsProcessed.WithLabelValues(collection, operation, "error").Inc()
				continue
			}

			// Record success metrics
			if collection != "" {
				telemetry.JetstreamRecordsProcessed.WithLabelValues(collection, operation, "success").Inc()
				telemetry.JetstreamProcessingDuration.WithLabelValues(collection, operation).Observe(time.Since(startTime).Seconds())
			}

			// Update cursor lag (time_us is microseconds since epoch)
			if msg.TimeUs > 0 {
				eventTime := time.UnixMicro(msg.TimeUs)
				lagSeconds := time.Since(eventTime).Seconds()
				if lagSeconds < 0 {
					lagSeconds = 0 // Future events shouldn't happen but handle gracefully
				}
				telemetry.JetstreamCursorLag.Set(lagSeconds)
			}
		}
	}
}

// Close closes the WebSocket connection
func (c *JetstreamClient) Close() error {
	telemetry.JetstreamConnectionStatus.Set(0)
	if c.conn != nil {
		err := c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Printf("Error sending close message: %v", err)
		}
		return c.conn.Close()
	}
	return nil
}

// RunWithReconnect runs the client with exponential backoff on connection errors
func RunWithReconnect(ctx context.Context, url string, queries *db.Queries) error {
	backoff := time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			client := NewJetstreamClient(url, queries)

			// Try to connect
			if err := client.Connect(ctx); err != nil {
				log.Printf("Connection error: %v. Retrying in %v...", err, backoff)
				telemetry.JetstreamReconnects.Inc()
				time.Sleep(backoff)

				// Exponential backoff
				backoff = backoff * 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}

			// Reset backoff on successful connection
			backoff = time.Second

			// Run the client
			if err := client.Run(ctx); err != nil {
				log.Printf("Runtime error: %v. Reconnecting...", err)
				telemetry.JetstreamReconnects.Inc()
				client.Close()
				time.Sleep(backoff)
				continue
			}

			// Clean shutdown
			client.Close()
			return nil
		}
	}
}
