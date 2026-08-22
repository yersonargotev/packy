package authorityphase

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

const (
	protocolVersion              = 1
	identityBytes                = 32
	maxProtocolBytes       int64 = 1 << 20
	candidateDirectoryName       = "candidate"
)

type prepareStatus string

const (
	prepareCandidate prepareStatus = "candidate"
	prepareResult    prepareStatus = "result"
)

type prepareRequest struct {
	Protocol      int                          `json:"protocol"`
	Token         string                       `json:"token"`
	Nonce         string                       `json:"nonce"`
	RequestSHA256 string                       `json:"request_sha256"`
	Request       managedpackpromotion.Request `json:"request"`
}

type prepareResponse struct {
	Protocol       int                            `json:"protocol"`
	Token          string                         `json:"token"`
	Nonce          string                         `json:"nonce"`
	RequestSHA256  string                         `json:"request_sha256"`
	Status         prepareStatus                  `json:"status"`
	Result         managedpackpromotion.Result    `json:"result"`
	Candidate      managedpackpromotion.Candidate `json:"candidate"`
	ResponseSHA256 string                         `json:"response_sha256"`
}

type publishRequest struct {
	Protocol      int                            `json:"protocol"`
	Token         string                         `json:"token"`
	Nonce         string                         `json:"nonce"`
	RequestSHA256 string                         `json:"request_sha256"`
	Candidate     managedpackpromotion.Candidate `json:"candidate"`
}

type publishResponse struct {
	Protocol       int                         `json:"protocol"`
	Token          string                      `json:"token"`
	Nonce          string                      `json:"nonce"`
	RequestSHA256  string                      `json:"request_sha256"`
	Result         managedpackpromotion.Result `json:"result"`
	ResponseSHA256 string                      `json:"response_sha256"`
}

func newPrepareRequest(request managedpackpromotion.Request) (prepareRequest, error) {
	token, err := randomIdentity()
	if err != nil {
		return prepareRequest{}, err
	}
	nonce, err := randomIdentity()
	if err != nil {
		return prepareRequest{}, err
	}
	value := prepareRequest{Protocol: protocolVersion, Token: token, Nonce: nonce, Request: request}
	value.RequestSHA256 = digest(value)
	return value, nil
}

func newPublishRequest(candidate managedpackpromotion.Candidate) (publishRequest, error) {
	token, err := randomIdentity()
	if err != nil {
		return publishRequest{}, err
	}
	nonce, err := randomIdentity()
	if err != nil {
		return publishRequest{}, err
	}
	value := publishRequest{Protocol: protocolVersion, Token: token, Nonce: nonce, Candidate: candidate}
	value.RequestSHA256 = digest(value)
	return value, nil
}

func randomIdentity() (string, error) {
	value := make([]byte, identityBytes)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", fmt.Errorf("create promotion process identity: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func writePrepareRequest(path string, request prepareRequest) error {
	return writeProtocol(path, request)
}
func writePublishRequest(path string, request publishRequest) error {
	return writeProtocol(path, request)
}

func readPrepareRequest(path string) (prepareRequest, error) {
	var request prepareRequest
	if err := readProtocol(path, &request); err != nil {
		return request, err
	}
	want := request.RequestSHA256
	request.RequestSHA256 = ""
	if request.Protocol != protocolVersion || !validIdentity(request.Token) || !validIdentity(request.Nonce) || want != digest(request) {
		return prepareRequest{}, errors.New("prepublication request identity or digest does not match its payload")
	}
	request.RequestSHA256 = want
	return request, nil
}

func readPublishRequest(path string) (publishRequest, error) {
	var request publishRequest
	if err := readProtocol(path, &request); err != nil {
		return request, err
	}
	want := request.RequestSHA256
	request.RequestSHA256 = ""
	if request.Protocol != protocolVersion || !validIdentity(request.Token) || !validIdentity(request.Nonce) || want != digest(request) {
		return publishRequest{}, errors.New("publication request identity or digest does not match its payload")
	}
	request.RequestSHA256 = want
	return request, nil
}

func writePrepareResponse(path string, request prepareRequest, response prepareResponse) error {
	response.Protocol, response.Token, response.Nonce, response.RequestSHA256 = request.Protocol, request.Token, request.Nonce, request.RequestSHA256
	response.ResponseSHA256 = ""
	response.ResponseSHA256 = digest(response)
	return writeProtocol(path, response)
}

func writePublishResponse(path string, request publishRequest, response publishResponse) error {
	response.Protocol, response.Token, response.Nonce, response.RequestSHA256 = request.Protocol, request.Token, request.Nonce, request.RequestSHA256
	response.ResponseSHA256 = ""
	response.ResponseSHA256 = digest(response)
	return writeProtocol(path, response)
}

func readPrepareResponse(path string, request prepareRequest) (prepareResponse, error) {
	var response prepareResponse
	if err := readProtocol(path, &response); err != nil {
		return response, err
	}
	want := response.ResponseSHA256
	response.ResponseSHA256 = ""
	if response.Protocol != request.Protocol || response.Token != request.Token || response.Nonce != request.Nonce || response.RequestSHA256 != request.RequestSHA256 {
		return prepareResponse{}, errors.New("prepublication response identity does not match the current request")
	}
	if want != digest(response) {
		return prepareResponse{}, errors.New("prepublication response digest does not match its payload")
	}
	response.ResponseSHA256 = want
	return response, nil
}

func readPublishResponse(path string, request publishRequest) (publishResponse, error) {
	var response publishResponse
	if err := readProtocol(path, &response); err != nil {
		return response, err
	}
	want := response.ResponseSHA256
	response.ResponseSHA256 = ""
	if response.Protocol != request.Protocol || response.Token != request.Token || response.Nonce != request.Nonce || response.RequestSHA256 != request.RequestSHA256 {
		return publishResponse{}, errors.New("publication response identity does not match the current request")
	}
	if want != digest(response) {
		return publishResponse{}, errors.New("publication response digest does not match its payload")
	}
	response.ResponseSHA256 = want
	return response, nil
}

func writeProtocol(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if int64(len(data)) > maxProtocolBytes {
		return fmt.Errorf("promotion process protocol exceeds %d bytes", maxProtocolBytes)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

func readProtocol(path string, destination any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxProtocolBytes {
		return errors.New("promotion process protocol must be a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("promotion process protocol contains trailing data")
	}
	return nil
}

func digest(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validIdentity(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == identityBytes
}

func pathWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
