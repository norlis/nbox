package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

// ✅ Banner ASCII.
const banner = `
▗▖  ▗▖▗▄▄▖  ▗▄▖ ▗▖  ▗▖    ▗▄▄▄▖▗▄▖  ▗▄▖ ▗▖    ▗▄▄▖
▐▛▚▖▐▌▐▌ ▐▌▐▌ ▐▌ ▝▚▞▘       █ ▐▌ ▐▌▐▌ ▐▌▐▌   ▐▌   
▐▌ ▝▜▌▐▛▀▚▖▐▌ ▐▌  ▐▌        █ ▐▌ ▐▌▐▌ ▐▌▐▌    ▝▀▚▖
▐▌  ▐▌▐▙▄▞▘▝▚▄▞▘▗▞▘▝▚▖      █ ▝▚▄▞▘▝▚▄▞▘▐▙▄▄▖▗▄▄▞▘
`

const (
	MsgToolDescription   = "       :: Password Hasher ::"
	MsgInputSeparator    = "...................................."
	MsgOutputSeparator   = "------------------------------------"
	MsgInputPrompt       = "› Ingresa la contraseña a hashear: "
	MsgSuccess           = "✅ Hash generado con éxito:"
	ErrEmptyInput        = "La contraseña no puede estar vacía."
	ErrFmtInvalidCost    = "Error: el costo debe estar entre %d y %d."
	ErrFmtReadInput      = "Error al leer la contraseña: %v"
	ErrFmtHashGeneration = "Error al generar el hash: %v"
)

func main() {
	fmt.Print(banner)

	cost := flag.Int("cost", bcrypt.DefaultCost, "Costo de Bcrypt (entre 4 y 31)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Herramienta para generar hashes de contraseñas usando bcrypt.\n\n")
		fmt.Fprintf(os.Stderr, "Uso: hasher [opciones]\n\n")
		fmt.Fprintf(os.Stderr, "Opciones:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *cost < bcrypt.MinCost || *cost > bcrypt.MaxCost {
		log.Fatalf(ErrFmtInvalidCost, bcrypt.MinCost, bcrypt.MaxCost)
	}

	fmt.Println(MsgToolDescription)
	fmt.Println(MsgInputSeparator)
	fmt.Println()

	fmt.Print(MsgInputPrompt)
	fmt.Println()

	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf(ErrFmtReadInput, err)
	}
	fmt.Println()

	password := strings.TrimSpace(string(passwordBytes))
	if password == "" {
		log.Fatal(ErrEmptyInput)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), *cost)
	if err != nil {
		log.Fatalf(ErrFmtHashGeneration, err)
	}

	border := "+" + strings.Repeat("-", 64) + "+"

	fmt.Println(MsgOutputSeparator)
	fmt.Println(MsgSuccess)

	// Imprimimos la caja con el hash dentro.
	fmt.Println(border)
	fmt.Printf("|  %s  |\n", string(hashedPassword))
	fmt.Println(border)
}
