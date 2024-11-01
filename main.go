package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

func handleHttp(res http.ResponseWriter, req *http.Request) {
	client := &http.Client{}
	destReq, err := http.NewRequest(req.Method, req.RequestURI, req.Body)
	if err != nil {
		http.Error(res, "Failed to create request", http.StatusInternalServerError)
		return
	}
	excludedHeaders := map[string]bool{
		"Connection":        true,
		"Content-Length":    true,
		"Transfer-Encoding": true,
		"Upgrade":           true,
	}
	for key, values := range req.Header {
		if !excludedHeaders[key] {
			for _, value := range values {
				destReq.Header.Add(key, value)
			}
		}
	}
	destRes, err := client.Do(destReq)
	if err != nil {
		http.Error(res, "Failed to reach target", http.StatusBadGateway)
		return
	}
	defer destRes.Body.Close()
	for key, values := range destRes.Header {
		if !excludedHeaders[key] {
			for _, value := range values {
				res.Header().Add(key, value)
			}
		}
	}
	res.WriteHeader(destRes.StatusCode)
	if _, err := io.Copy(res, destRes.Body); err != nil {
		log.Printf("Failed to copy response body: %v", err)
	}
}

func handleHttps(res http.ResponseWriter, req *http.Request) {
	host := req.Host
	if !strings.Contains(host, ":") {
		host += ":443"
	}

	destConn, err := net.Dial("tcp", host)
	if err != nil {
		log.Printf("Connection error: %v", err)
		http.Error(res, "Connection failed", http.StatusServiceUnavailable)
		return
	}
	defer destConn.Close()
	res.WriteHeader(http.StatusOK)

	hijacker, ok := res.(http.Hijacker)
	if !ok {
		http.Error(res, "Hijacking unsupported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Printf("Hijack error: %v", err)
		http.Error(res, "Hijack failed", http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	done := make(chan struct{}, 2)
	go func() { io.Copy(destConn, clientConn); done <- struct{}{} }()
	go func() { io.Copy(clientConn, destConn); done <- struct{}{} }()
	<-done
	<-done
}

func main() {
	port := flag.String("port", "8080", "port to listen on")
	flag.Parse()

	server := &http.Server{
		Addr: fmt.Sprintf(":%s", *port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				handleHttps(w, r)
			} else {
				handleHttp(w, r)
			}
		}),
	}

	log.Printf("Starting proxy server on port %s...", *port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
