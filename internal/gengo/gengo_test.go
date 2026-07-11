package gengo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/1homsi/onekit/internal/onkcompile"
	"github.com/1homsi/onekit/internal/onkir"
	"github.com/1homsi/onekit/internal/onklang"
)

const fixtureSrc = `
package main

message User {
  id: string
  name: string @len(2, 100)
  email: string @email
}

message CreateUserRequest {
  name: string @len(2, 100)
  email: string @email
}

message GetUserRequest {
  id: string
}

message NotFoundError @status(404) {
  resource_type: string
  resource_id: string
}

service UserService {
  base_path: "/api/v1"

  createUser(CreateUserRequest) -> User @post("/users")

  getUser(GetUserRequest) -> User | NotFoundError @get("/users/{id}")
}
`

func compileFixture(t *testing.T) *onkir.File {
	t.Helper()
	ast, err := onklang.Parse(fixtureSrc)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	pkg, err := onkcompile.Compile([]onkcompile.Source{{Path: "app.onk", AST: ast}})
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	return pkg.Files[0]
}

func TestGenerateTypesFormatsCleanly(t *testing.T) {
	file := compileFixture(t)
	out, err := GenerateTypes(file)
	if err != nil {
		t.Fatalf("GenerateTypes error: %v\n%s", err, out)
	}
	if len(out) == 0 {
		t.Fatalf("expected non-empty generated types")
	}
}

func TestGenerateValidationFormatsCleanly(t *testing.T) {
	file := compileFixture(t)
	out, err := GenerateValidation(file)
	if err != nil {
		t.Fatalf("GenerateValidation error: %v\n%s", err, out)
	}
	if len(out) == 0 {
		t.Fatalf("expected non-empty generated validation code")
	}
}

func TestGenerateServerFormatsCleanly(t *testing.T) {
	file := compileFixture(t)
	out, err := GenerateServer(file)
	if err != nil {
		t.Fatalf("GenerateServer error: %v\n%s", err, out)
	}
	if len(out) == 0 {
		t.Fatalf("expected non-empty generated server code")
	}
}

const harnessMain = `
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
)

type impl struct {
	users map[string]*User
}

func (s *impl) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
	u := &User{Id: fmt.Sprintf("user-%d", len(s.users)+1), Name: req.Name, Email: req.Email}
	s.users[u.Id] = u
	return u, nil
}

func (s *impl) GetUser(ctx context.Context, req *GetUserRequest) (*User, error) {
	u, ok := s.users[req.Id]
	if !ok {
		return nil, &NotFoundError{ResourceType: "user", ResourceId: req.Id}
	}
	return u, nil
}

func fail(msg string, args ...any) {
	fmt.Printf(msg+"\n", args...)
	os.Exit(1)
}

func main() {
	mux := http.NewServeMux()
	RegisterUserServiceServer(mux, &impl{users: map[string]*User{}})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"name": "Ada Lovelace", "email": "ada@example.com"})
	resp, err := http.Post(srv.URL+"/api/v1/users", "application/json", bytes.NewReader(body))
	if err != nil {
		fail("create request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		fail("expected 200 from create, got %d", resp.StatusCode)
	}
	var created User
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		fail("decode create response: %v", err)
	}
	if created.Name != "Ada Lovelace" || created.Email != "ada@example.com" || created.Id == "" {
		fail("unexpected created user: %+v", created)
	}

	getResp, err := http.Get(srv.URL + "/api/v1/users/" + created.Id)
	if err != nil {
		fail("get request failed: %v", err)
	}
	if getResp.StatusCode != 200 {
		fail("expected 200 from get, got %d", getResp.StatusCode)
	}
	var fetched User
	if err := json.NewDecoder(getResp.Body).Decode(&fetched); err != nil {
		fail("decode get response: %v", err)
	}
	if fetched.Id != created.Id || fetched.Name != created.Name {
		fail("unexpected fetched user: %+v", fetched)
	}

	missResp, err := http.Get(srv.URL + "/api/v1/users/does-not-exist")
	if err != nil {
		fail("missing-user request failed: %v", err)
	}
	if missResp.StatusCode != 404 {
		fail("expected 404 for missing user, got %d", missResp.StatusCode)
	}
	var notFound NotFoundError
	if err := json.NewDecoder(missResp.Body).Decode(&notFound); err != nil {
		fail("decode not-found response: %v", err)
	}
	if notFound.ResourceType != "user" || notFound.ResourceId != "does-not-exist" {
		fail("unexpected not-found body: %+v", notFound)
	}

	badBody, _ := json.Marshal(map[string]string{"name": "A", "email": "not-an-email"})
	badResp, err := http.Post(srv.URL+"/api/v1/users", "application/json", bytes.NewReader(badBody))
	if err != nil {
		fail("bad create request failed: %v", err)
	}
	if badResp.StatusCode != 400 {
		fail("expected 400 for invalid create, got %d", badResp.StatusCode)
	}

	fmt.Println("OK")
}
`

func TestGeneratedServerBuildsAndServes(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	file := compileFixture(t)

	typesSrc, err := GenerateTypes(file)
	if err != nil {
		t.Fatalf("GenerateTypes error: %v", err)
	}
	validateSrc, err := GenerateValidation(file)
	if err != nil {
		t.Fatalf("GenerateValidation error: %v", err)
	}
	serverSrc, err := GenerateServer(file)
	if err != nil {
		t.Fatalf("GenerateServer error: %v", err)
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module onekit_gengo_fixture\n\ngo 1.26\n")
	writeFile(t, filepath.Join(dir, "types.go"), string(typesSrc))
	writeFile(t, filepath.Join(dir, "validate.go"), string(validateSrc))
	writeFile(t, filepath.Join(dir, "server.go"), string(serverSrc))
	writeFile(t, filepath.Join(dir, "main.go"), harnessMain)

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated program failed: %v\n%s", err, out)
	}
	if got := string(out); got != "OK\n" {
		t.Fatalf("unexpected program output: %q", got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
