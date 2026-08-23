package offlinevalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

const (
	protocolVersion = 1
	tokenBytes      = 32
	nonceBytes      = 32
)

type responseStatus string

const (
	responseAccepted responseStatus = "accepted"
	responseRejected responseStatus = "rejected"
)

type workerRequest struct {
	Protocol      int               `json:"protocol"`
	Token         string            `json:"token"`
	Nonce         string            `json:"nonce"`
	RequestSHA256 string            `json:"request_sha256"`
	ProjectRoot   string            `json:"project_root"`
	OriginRoots   map[string]string `json:"origin_roots"`
}

type workerResponse struct {
	Protocol        int                           `json:"protocol"`
	Token           string                        `json:"token"`
	Nonce           string                        `json:"nonce"`
	RequestSHA256   string                        `json:"request_sha256"`
	Status          responseStatus                `json:"status"`
	Gate            managedpackpromotion.Gate     `json:"gate,omitempty"`
	Reason          string                        `json:"reason,omitempty"`
	Preflight       managedpack.PreflightEvidence `json:"preflight"`
	PreflightSHA256 string                        `json:"preflight_sha256,omitempty"`
	ResponseSHA256  string                        `json:"response_sha256"`
}

func requestDigest(request workerRequest) string {
	request.RequestSHA256 = ""
	return jsonDigest(request)
}

func preflightDigest(preflight managedpack.PreflightEvidence) string {
	return jsonDigest(preflight)
}

func responseDigest(response workerResponse) string {
	response.ResponseSHA256 = ""
	return jsonDigest(response)
}

func jsonDigest(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
