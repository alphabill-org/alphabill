package backend

import (
	"github.com/go-playground/validator/v10"
)

// use a single instance of Validate, it caches struct info
var validate *validator.Validate

func registerValidator() {
	validate = validator.New()
}
