package main

import (
	"fmt"
	"log"
	"os"

	"github.com/emersion/go-imap/client"
)

func main() {
    username := os.Getenv("EMAIL_USERNAME")
    password := os.Getenv("EMAIL_PASSWORD")

    // Conectar al servidor IMAP
    c, err := client.DialTLS("imap.gmail.com:993", nil) // Cambia esto según tu servidor IMAP
    if err != nil {
        log.Fatal(err)
    }
    defer c.Logout()

    // Iniciar sesión
    if err := c.Login(username, password); err != nil {
        log.Fatal(err)
    }
    fmt.Println("Conexión IMAP exitosa!")
}
