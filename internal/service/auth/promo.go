package auth

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"crypto/rand"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/mongo"
	"log/slog"
	"math/big"
	"time"
)

func (s *Service) GeneratePromoCodes(number int) error {
	generatedCodes := make([]string, 0, number)
	attempts := 0

	for len(generatedCodes) < number {
		// Safety: avoid infinite loop if repository is huge
		if attempts > number*10 {
			return fmt.Errorf("failed to generate enough unique promo codes")
		}
		attempts++

		// Generate a random code (8 characters)
		code := generateRandomCode(8)

		// Check if it already exists in DB
		existing, err := s.repository.GetPromoCode(code)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("failed to check existing code: %w", err)
		}
		if existing != nil {
			continue // Code already exists, generate a new one
		}

		// Code is unique: add to list
		generatedCodes = append(generatedCodes, code)
	}

	// Save all generated codes in DB
	return s.repository.SavePromoCodes(generatedCodes)
}

func generateRandomCode(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func (s *Service) GenerateRandomNumCode(length int) string {
	const charset = "0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func (s *Service) GetActivePromoCodes() ([]entity.PromoCode, error) {
	codes, err := s.repository.GetAllPromoCodes()
	if err != nil {
		return nil, fmt.Errorf("failed to get promo codes: %w", err)
	}

	if len(codes) == 0 {
		return nil, nil // No active promo codes
	}

	var activeCodes []entity.PromoCode
	for _, code := range codes {
		if !code.Activated {
			activeCodes = append(activeCodes, code)
		}
	}

	return activeCodes, nil
}

func (s *Service) ActivatePromoCode(phone, code string) error {
	if entity.NormalizePhone(phone) == "" {
		return fmt.Errorf("phone number is required")
	}

	user, err := s.GetUser("", phone, 0)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	promoCode, err := s.repository.GetPromoCode(code)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("failed to check existing code: %w", err)
	}
	if promoCode == nil {
		return fmt.Errorf("promo code %s does not exist", code)
	}
	if promoCode.Activated {
		return fmt.Errorf("promo code %s is already activated", code)
	}

	err = s.repository.ActivatePromoCode(promoCode.Code)
	if err != nil {
		return fmt.Errorf("failed to activate promo code: %w", err)
	}

	// The code is claimed atomically above; if granting access fails, release it again
	// so the customer doesn't lose the code. (No multi-document transaction: the
	// deployment is not guaranteed to be a replica set.)
	err = s.UpdateUserFields(user, map[string]any{
		entity.UserFieldPromoExpire: time.Now().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		if rbErr := s.repository.DeactivatePromoCode(promoCode.Code); rbErr != nil {
			s.log.Error("releasing promo code after failed grant",
				slog.String("code", promoCode.Code), sl.Err(rbErr))
		}
		return err
	}

	return nil
}
