package kyb

import "github.com/HernanEspinozaDev/espaciGo/internal/verification"

func NormalizeChileanRUT(value string) (string, error) {
	return verification.NormalizeRut(value)
}

func ValidateChileanRUT(value string) bool {
	return verification.ValidateRut(value)
}
