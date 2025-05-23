package pgpasswd

// @see https://github.com/postgres/postgres/blob/c30f54ad732ca5c8762bb68bbe0f51de9137dd72/src/interfaces/libpq/fe-auth.c#L1167-L1285
// @see https://github.com/postgres/postgres/blob/e6bdfd9700ebfc7df811c97c2fc46d7e94e329a2/src/interfaces/libpq/fe-auth-scram.c#L868-L905
// @see https://github.com/postgres/postgres/blob/c30f54ad732ca5c8762bb68bbe0f51de9137dd72/src/port/pg_strong_random.c#L66-L96
// @see https://github.com/postgres/postgres/blob/e6bdfd9700ebfc7df811c97c2fc46d7e94e329a2/src/common/scram-common.c#L160-L274
// @see https://github.com/postgres/postgres/blob/e6bdfd9700ebfc7df811c97c2fc46d7e94e329a2/src/common/scram-common.c#L27-L85

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// @see https://github.com/postgres/postgres/blob/e6bdfd9700ebfc7df811c97c2fc46d7e94e329a2/src/include/common/scram-common.h#L36-L41
	saltSize = 16

	// @see https://github.com/postgres/postgres/blob/c30f54ad732ca5c8762bb68bbe0f51de9137dd72/src/include/common/sha2.h#L22
	digestLen = 32

	// @see https://github.com/postgres/postgres/blob/e6bdfd9700ebfc7df811c97c2fc46d7e94e329a2/src/include/common/scram-common.h#L43-L47
	iterationCnt = 4096
)

var (
	clientRawKey = []byte("Client Key")
	serverRawKey = []byte("Server Key")
)

func genSalt(size int) ([]byte, error) {
	salt := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}

func encodeB64(src []byte) (dst []byte) {
	dst = make([]byte, base64.StdEncoding.EncodedLen(len(src)))
	base64.StdEncoding.Encode(dst, src)
	return
}

func getHMACSum(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(msg)
	return h.Sum(nil)
}

func getSHA256Sum(key []byte) []byte {
	h := sha256.New()
	_, _ = h.Write(key)
	return h.Sum(nil)
}

func encrypt(raw, salt []byte, iter, keyLen int) string {
	digestKey := pbkdf2.Key(raw, salt, iter, keyLen, sha256.New)
	clientKey := getHMACSum(digestKey, clientRawKey)
	storedKey := getSHA256Sum(clientKey)
	serverKey := getHMACSum(digestKey, serverRawKey)

	return fmt.Sprintf(
		"SCRAM-SHA-256$%d:%s$%s:%s",
		iter,
		string(encodeB64(salt)),
		string(encodeB64(storedKey)),
		string(encodeB64(serverKey)),
	)
}

// Encrypt encrypts a raw password with scram-sha-256
func Encrypt(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	salt, err := genSalt(saltSize)
	if err != nil {
		return "", err
	}

	return encrypt(raw, salt, iterationCnt, digestLen), nil
}

// Check if scram is a scram-sha-256 of raw
func Check(scram string, raw []byte) (bool, error) {
	if scram == "" {
		return false, errors.New("empty scram")
	}

	s, ok := strings.CutPrefix(scram, "SCRAM-SHA-256$")
	if !ok {
		return false, errors.New("no SCRAM-SHA-256 prefix")
	}

	s, _, ok = strings.Cut(s, "$")
	if !ok {
		return false, errors.New("missing $")
	}

	iterationStr, salt64, ok := strings.Cut(s, ":")
	if !ok {
		return false, errors.New("missing :")
	}

	iterations, err := strconv.Atoi(iterationStr)

	salt, err := base64.StdEncoding.DecodeString(salt64)
	if err != nil {
		return false, fmt.Errorf("base64 salt: %w", err)
	}

	return encrypt(raw, salt, iterations, digestLen) == scram, nil
}
