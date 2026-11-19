package model

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"

	"gorm.io/gorm"
)

const ExternalIdentityProviderGoogle = "google"

var (
	// ErrGoogleEmailBelongsToAccount blocks Google sign-up when the email is
	// already used by any account, including deleted ones. Accounts are never
	// merged by email: the owner must sign in and link Google explicitly.
	ErrGoogleEmailBelongsToAccount = errors.New("google email belongs to an existing account")
	ErrGoogleAlreadyLinked         = errors.New("account already has a linked Google identity")
)

// FindUserByExternalIdentity returns the live user that owns a provider
// subject, or gorm.ErrRecordNotFound when the subject is unclaimed.
func FindUserByExternalIdentity(provider, subject string) (*User, error) {
	var claim ExternalIdentityClaim
	if err := DB.Where("provider = ? AND subject = ?", provider, subject).First(&claim).Error; err != nil {
		return nil, err
	}
	var user User
	if err := DB.First(&user, claim.UserId).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// HasExternalIdentity reports whether the user has claimed a provider slot.
func HasExternalIdentity(userID int, provider string) (bool, error) {
	var count int64
	err := DB.Model(&ExternalIdentityClaim{}).Where("provider = ? AND user_id = ?", provider, userID).Count(&count).Error
	return count > 0, err
}

// CreateGoogleUser creates a password-less account bound to a Google subject
// in one transaction. The new-user grant is applied by InsertWithTx; callers
// must call FinishInsert after this returns.
func CreateGoogleUser(subject, email, name string) (*User, error) {
	email = NormalizeEmail(email)
	if strings.TrimSpace(subject) == "" || email == "" {
		return nil, errors.New("google identity is incomplete")
	}
	user := &User{Role: common.RoleCommonUser, Email: email}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := emailQuery(tx, email).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return ErrGoogleEmailBelongsToAccount
		}
		username, err := uniqueGoogleUsernameWithTx(tx, email)
		if err != nil {
			return err
		}
		user.Username = username
		user.DisplayName = googleDisplayName(name, username)
		if err := user.InsertWithTx(tx); err != nil {
			if errors.Is(err, ErrEmailAlreadyTaken) {
				return ErrGoogleEmailBelongsToAccount
			}
			return err
		}
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderGoogle, subject, user.Id)
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

// BindGoogleForSessionWithTx links a Google subject to the signed-in user.
// The session must still be the one that started the flow.
func BindGoogleForSessionWithTx(tx *gorm.DB, identity AuthSessionIdentity, subject string) error {
	if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
		return err
	}
	var count int64
	if err := tx.Model(&ExternalIdentityClaim{}).
		Where("provider = ? AND user_id = ?", ExternalIdentityProviderGoogle, identity.UserID).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrGoogleAlreadyLinked
	}
	return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderGoogle, subject, identity.UserID)
}

// uniqueGoogleUsernameWithTx derives a readable username from the email local
// part plus a random suffix. Deleted users keep their usernames reserved.
func uniqueGoogleUsernameWithTx(tx *gorm.DB, email string) (string, error) {
	local := email
	if at := strings.IndexByte(email, '@'); at > 0 {
		local = email[:at]
	}
	var base strings.Builder
	for _, r := range strings.ToLower(local) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			base.WriteRune(r)
		}
		if base.Len() == 12 {
			break
		}
	}
	prefix := base.String()
	if prefix == "" {
		prefix = "user"
	}
	for attempt := 0; attempt < 5; attempt++ {
		candidate := prefix + "_" + strings.ToLower(common.GetRandomString(6))
		var count int64
		if err := tx.Unscoped().Model(&User{}).Where("username = ?", candidate).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
	}
	return "", errors.New("could not allocate a unique username")
}

func googleDisplayName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback
	}
	if utf8.RuneCountInString(name) > 20 {
		name = string([]rune(name)[:20])
	}
	return name
}
