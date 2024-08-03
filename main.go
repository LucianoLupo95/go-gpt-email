package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"github.com/joho/godotenv"
)

func main() {
    err := godotenv.Load()
    if err != nil {
        log.Fatal("Error cargando el archivo .env")
    }
    username := os.Getenv("EMAIL_USERNAME")
    password := os.Getenv("EMAIL_PASSWORD")

      // Función para conectar al servidor IMAP
      connectAndFetchEmails := func() {
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
        //Seleccionar la bandeja de entrada
        _, err = c.Select("INBOX", false)
        if err != nil {
            log.Fatal(err)
        }

        // Buscar correos no leídos con el asunto "gpt-go-email"
        criteria := imap.NewSearchCriteria()
        criteria.WithoutFlags = []string{"\\Seen"}
        criteria.Header.Add("Subject", "gpt-go-email")
        ids, err := c.Search(criteria)
        if err != nil {
            log.Fatal(err)
        }

        if len(ids) == 0 {
            fmt.Println("No se encontraron correos con el asunto 'gpt-go-email'.")
            return
        }

        // Obtener los correos encontrados
        seqset := new(imap.SeqSet)
        seqset.AddNum(ids...)
        messages := make(chan *imap.Message, 10)
        go func() {
            section := &imap.BodySectionName{}
            items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchBody, section.FetchItem()}
            if err := c.Fetch(seqset, items, messages); err != nil {
                log.Fatal(err)
            }
        }()

        for msg := range messages {
            processMessage(msg)
        }
    }
       // Loop para verificar correos nuevos cada 5 minutos
       ticker := time.NewTicker(10 * time.Second)
       defer ticker.Stop()

    // Uso de for range para iterar sobre los ticks
    for range ticker.C {
        fmt.Println("TICK")
        connectAndFetchEmails()
    }
}
func processMessage(msg *imap.Message) {
    if msg.Envelope == nil || len(msg.Envelope.From) == 0 {
        fmt.Println("El mensaje no tiene un remitente.")
        return
    }

    // Construir la dirección de correo del remitente
    from := msg.Envelope.From[0].MailboxName + "@" + msg.Envelope.From[0].HostName
    subject := msg.Envelope.Subject
    fmt.Printf("Procesando mensaje de %s con asunto: %s\n", from, subject)

    // Leer el cuerpo del mensaje
    if msg.Body == nil {
        fmt.Println("No se pudo leer el cuerpo del mensaje.")
        return
    }

    section := &imap.BodySectionName{}
    body := msg.GetBody(section)
    if body == nil {
        fmt.Println("El mensaje no contiene un cuerpo válido.")
        return
    }

    mr, err := mail.CreateReader(body)
    if err != nil {
        log.Fatal(err)
    }

    var bodyText string
    for {
        part, err := mr.NextPart()
        if err == io.EOF {
            break
        }
        if err != nil {
            log.Fatal(err)
        }

        contentType := part.Header.Get("Content-Type")
        mediaType, _, err := mime.ParseMediaType(contentType)
        if err != nil {
            log.Fatal(err)
        }

        if strings.HasPrefix(mediaType, "text/plain") {
            b, err := io.ReadAll(part.Body)
            if err != nil {
                log.Fatal(err)
            }
            bodyText = string(b)
        }
    }
    response, err := callChatGPT(bodyText)
    if err != nil {
        log.Printf("Error llamando a ChatGPT: %v", err)
        response = "Hubo un error en ChatGPT. Disculpe la demora."
    }

    // Enviar una respuesta automática con la respuesta de ChatGPT
    err = sendAutoReply(from, response)
    if err != nil {
        log.Printf("Error enviando respuesta automática: %v", err)
    }
}

func sendAutoReply(to string, response string) error {
    smtpServer := os.Getenv("SMTP_SERVER")
    smtpPort := os.Getenv("SMTP_PORT")
    smtpUsername := os.Getenv("SMTP_USERNAME")
    smtpPassword := os.Getenv("SMTP_PASSWORD")

    auth := smtp.PlainAuth("", smtpUsername, smtpPassword, smtpServer)
    from := smtpUsername
    msg := []byte("To: " + to + "\r\n" +
        "Subject: Re:gpt-go-email\r\n" +
        "\r\n" +
        response + "\r\n")

    addr := fmt.Sprintf("%s:%s", smtpServer, smtpPort)
    err := smtp.SendMail(addr, auth, from, []string{to}, msg)
    return err
}

const (
    openAIURL = "https://api.openai.com/v1/chat/completions"
)

func callChatGPT(prompt string) (string, error) {
    apiKey := os.Getenv("OPENAI_API_KEY")

    requestBody, err := json.Marshal(map[string]interface{}{
        "model": "gpt-3.5-turbo",
        "messages": []map[string]string{
            {"role": "user", "content": prompt},
        },
    })
    if err != nil {
        return "", fmt.Errorf("error creando el cuerpo de la solicitud: %v", err)
    }

    req, err := http.NewRequest("POST", openAIURL, bytes.NewBuffer(requestBody))
    if err != nil {
        return "", fmt.Errorf("error creando la solicitud: %v", err)
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+apiKey)

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("error en la solicitud: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := ioutil.ReadAll(resp.Body)
        return "", fmt.Errorf("error en la solicitud: %s", body)
    }

    var response map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return "", fmt.Errorf("error decodificando la respuesta: %v", err)
    }

    choices, ok := response["choices"].([]interface{})
    if !ok || len(choices) == 0 {
        return "", fmt.Errorf("no se recibieron respuestas de ChatGPT")
    }

    content, ok := choices[0].(map[string]interface{})["message"].(map[string]interface{})["content"].(string)
    if !ok {
        return "", fmt.Errorf("error extrayendo el contenido de la respuesta")
    }

    return content, nil
}
