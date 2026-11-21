package aws

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Credentials holds AWS signing credentials.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string // #nosec G117 -- runtime AWS credential
	SessionToken    string
}

// LoadCredentials reads AWS credentials from environment variables or
// ~/.aws/credentials for the active profile.
func LoadCredentials() Credentials {
	creds := Credentials{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
	}
	if creds.AccessKeyID != "" && creds.SecretAccessKey != "" {
		return creds
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return creds
	}
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "default"
	}
	fileCreds := loadCredentialsFromFile(filepath.Join(home, ".aws", "credentials"), profile)
	if creds.AccessKeyID == "" {
		creds.AccessKeyID = fileCreds.AccessKeyID
	}
	if creds.SecretAccessKey == "" {
		creds.SecretAccessKey = fileCreds.SecretAccessKey
	}
	if creds.SessionToken == "" {
		creds.SessionToken = fileCreds.SessionToken
	}
	return creds
}

func loadCredentialsFromFile(path, profile string) Credentials {
	f, err := os.Open(filepath.Clean(path)) // #nosec G304 -- standard AWS credentials path in user home
	if err != nil {
		return Credentials{}
	}
	defer f.Close()

	var creds Credentials
	inSection := false
	target := "[" + profile + "]"

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = line == target
			continue
		}
		if !inSection {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(strings.ToLower(k))
		v = strings.TrimSpace(v)
		switch k {
		case "aws_access_key_id":
			creds.AccessKeyID = v
		case "aws_secret_access_key":
			creds.SecretAccessKey = v
		case "aws_session_token":
			creds.SessionToken = v
		}
	}
	return creds
}

// SignRequest signs an HTTP request using AWS Signature Version 4.
func SignRequest(req *http.Request, body []byte, creds Credentials, region, service string, now time.Time) {
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return
	}
	utc := now.UTC()
	amzDate := utc.Format("20060102T150405Z")
	dateStamp := utc.Format("20060102")

	payloadHash := sha256Hex(body)
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if creds.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.SessionToken)
	}

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := req.URL.Query().Encode()

	headerNames := make([]string, 0, len(req.Header))
	lowerHeaders := make(map[string]string, len(req.Header))
	for k, vals := range req.Header {
		lk := strings.ToLower(k)
		headerNames = append(headerNames, lk)
		lowerHeaders[lk] = strings.TrimSpace(strings.Join(vals, ","))
	}
	sort.Strings(headerNames)

	var canonicalHeaders strings.Builder
	for _, k := range headerNames {
		canonicalHeaders.WriteString(k)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(lowerHeaders[k])
		canonicalHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(headerNames, ";")

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(creds.SecretAccessKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	authHeader := "AWS4-HMAC-SHA256 Credential=" + creds.AccessKeyID + "/" + scope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature
	req.Header.Set("Authorization", authHeader)
}

func deriveSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
