package debuglog

import (
	"fmt"
	"log"
	"os"
)

var logger *log.Logger

// Init abre (ou cria) o arquivo de log e ativa o logging de debug.
// Se path estiver vazio, usa "debug.log" no diretório corrente.
func Init(path string) error {
	if path == "" {
		path = "debug.log"
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("debuglog: não foi possível abrir %s: %w", path, err)
	}
	logger = log.New(f, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	logger.Println("=== debug session start ===")
	return nil
}

// Log escreve uma linha de debug se o logging estiver ativo.
func Log(format string, args ...any) {
	if logger == nil {
		return
	}
	logger.Printf(format, args...)
}

// Enabled retorna true se o logging estiver ativo.
func Enabled() bool {
	return logger != nil
}
