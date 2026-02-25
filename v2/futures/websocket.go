package futures

import (
	"log"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WsHandler handle raw websocket message
type WsHandler func(message []byte)

// ErrHandler handles errors
type ErrHandler func(err error)

// WsConfig webservice configuration
type WsConfig struct {
	Endpoint string
	Proxy    *string
}

func newWsConfig(endpoint string) *WsConfig {
	return &WsConfig{
		Endpoint: endpoint,
		Proxy:    getWsProxyUrl(),
	}
}

var wsServe = func(cfg *WsConfig, handler WsHandler, errHandler ErrHandler) (doneC, stopC chan struct{}, err error) {
	proxy := http.ProxyFromEnvironment
	if cfg.Proxy != nil {
		u, err := url.Parse(*cfg.Proxy)
		if err != nil {
			return nil, nil, err
		}
		proxy = http.ProxyURL(u)
	}
	Dialer := websocket.Dialer{
		Proxy:             proxy,
		HandshakeTimeout:  45 * time.Second,
		EnableCompression: true,
	}

	c, _, err := Dialer.Dial(cfg.Endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	c.SetReadLimit(655350)
	doneC = make(chan struct{})
	stopC = make(chan struct{})
	go func() {
		// This function will exit either on error from
		// websocket.Conn.ReadMessage or when the stopC channel is
		// closed by the client.
		// defer close(doneC)
		defer func() {
			close(doneC)
			log.Println("wsServe done closed")
		}()
		if WebsocketKeepalive {
			keepAlive(doneC, c, WebsocketTimeout)
		}
		// Wait for the stopC channel to be closed.  We do that in a
		// separate goroutine because ReadMessage is a blocking
		// operation.
		silent := false
		go func() {
			select {
			case <-stopC:
				silent = true
			case <-doneC:
			}
			c.Close()
			log.Println("wsServe websocket.Conn.Closed")
		}()
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Printf("wsServe ReadMessage failed: %s", err)
				if !silent {
					errHandler(err)
				}
				return
			}
			handler(message)
		}
	}()
	return
}

func keepAlive(doneC chan struct{}, c *websocket.Conn, timeout time.Duration) {

	var lastResponse atomic.Int64
	lastResponse.Store(time.Now().UnixMilli())

	c.SetPingHandler(func(pingData string) error {
		log.Println("wsServe PingHandler")
		// Respond with Pong using the server's PING payload
		err := c.WriteControl(
			websocket.PongMessage,
			[]byte(pingData),
			time.Now().Add(WebsocketPongTimeout), // Short deadline to ensure timely response
		)
		if err != nil {
			log.Printf("wsServe PingHandler failed: %s", err)
			return err
		}

		lastResponse.Store(time.Now().UnixMilli())

		return nil
	})

	ticker := time.NewTicker(timeout)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-doneC:
				log.Println("wsServe keepAlive done")
				return
			case <-ticker.C:
				last := lastResponse.Load()
				if time.Since(time.UnixMilli(last)) > timeout {
					c.Close()
					return
				}

			}
		}
	}()
}

var WsGetReadWriteConnection = func(cfg *WsConfig) (*websocket.Conn, error) {
	proxy := http.ProxyFromEnvironment
	if cfg.Proxy != nil {
		u, err := url.Parse(*cfg.Proxy)
		if err != nil {
			return nil, err
		}
		proxy = http.ProxyURL(u)
	}

	Dialer := websocket.Dialer{
		Proxy:             proxy,
		HandshakeTimeout:  45 * time.Second,
		EnableCompression: false,
	}

	c, _, err := Dialer.Dial(cfg.Endpoint, nil)
	if err != nil {
		return nil, err
	}

	return c, nil
}
