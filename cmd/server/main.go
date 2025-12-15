package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	internal "github.com/lautaroblasco23/imagestore/internal"
	pb "github.com/lautaroblasco23/imagestore/proto/imagestore/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	dbPath    = "./imagestore.db"
	imagesDir = "./images"
)

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	baseURL := getEnv("BASE_URL", "http://localhost:8087")

	bindAddr := getEnv("BIND_ADDR", "127.0.0.1")
	grpcAddr := bindAddr + ":50051"
	httpAddr := bindAddr + ":8087"

	if err := os.MkdirAll(imagesDir, 0o750); err != nil {
		log.Fatalf("failed to create images directory: %v", err)
	}

	db, err := internal.NewDB(dbPath)
	if err != nil {
		log.Fatalf("failed to create database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("error closing database: %v", err)
		}
	}()

	storage := internal.NewStorage(imagesDir)
	handler := internal.NewImageHandler(db, storage, baseURL)

	grpcServer := grpc.NewServer()
	pb.RegisterImageServiceServer(grpcServer, handler)
	reflection.Register(grpcServer)

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/images/", handler.ServeHTTP)
	httpMux.HandleFunc("/health", handler.HealthCheck)

	go func() {
		listener, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			log.Fatalf("failed to listen on %s: %v", grpcAddr, err)
		}
		log.Printf("gRPC server listening on %s", grpcAddr)
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("failed to serve gRPC: %v", err)
		}
	}()

	go func() {
		httpServer := &http.Server{
			Addr:         httpAddr,
			Handler:      httpMux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		log.Printf("HTTP server listening on %s", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("failed to serve HTTP: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down servers...")
	grpcServer.GracefulStop()
	log.Println("servers stopped")
}
