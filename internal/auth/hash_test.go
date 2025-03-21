package auth

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestMatchPassword(t *testing.T) {
	userInput := "Testing@123"
	hash, err := HashPassword(userInput)
	if err != nil {
		t.Errorf("found error in HashPassword: %v", err)
	}
	err = CheckPasswordHash(userInput, hash)
	if err != nil {
		t.Errorf("found error in HashPassword: %v", err)
	} else {
		fmt.Print("Test passed for password match")
	}
}

func TestMismatchPassword(t *testing.T) {
	userInput := "Testing@123"
	hash, err := HashPassword(userInput)
	if err != nil {
		t.Errorf("Found error in HashPassword: %v", err)
	}
	userInput = "NewPassword"
	err = CheckPasswordHash(userInput, hash)

	if err != nil {
		fmt.Printf("Found error in HashPassword: %v", err)
	} else {
		t.Errorf("test failed for mismatch password")
	}
}

func TestJWTSuccess(t *testing.T) {
	userId := uuid.New()
	secret := "testingSecret"

	token, err := MakeJWT(userId, secret)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, token)
	}

	tokenUserId, err := ValidateJWT(token, secret)
	assert.NoError(t, err)
	assert.Equal(t, userId, tokenUserId)
}

func TestJWTExpiry(t *testing.T) {
	userId := uuid.New()
	secret := "testingSecret"

	token, err := MakeJWT(userId, secret)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, token)
	}

	time.Sleep(time.Millisecond * 11)

	_, err = ValidateJWT(token, secret)
	assert.Error(t, err)
}

func TestIncorrectToken(t *testing.T) {
	userId := uuid.New()
	secret := "testingSecret"

	token, err := MakeJWT(userId, secret)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, token)
	}

	token += "gibberish"
	_, err = ValidateJWT(token, secret)
	assert.Error(t, err)
}
