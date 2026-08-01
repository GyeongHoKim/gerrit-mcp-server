package mcpserver

import (
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/version"
)

// newGerrit returns a client pointed at a host that is never contacted. Tool
// listing needs a client to build the server, not to reach one.
func newGerrit(t *testing.T) *gerrit.Client {
	t.Helper()

	base, err := url.Parse("https://gerrit.example.com")
	if err != nil {
		t.Fatalf("parsing base url: %v", err)
	}

	return gerrit.New(gerrit.Options{
		BaseURL: base,
		User:    "alice",
		Token:   "s3cret",
		Timeout: time.Second,
	})
}

// connect wires a client to srv over an in-memory transport and returns the
// established session.
func connect(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	serverSession, err := srv.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connecting server: %v", err)
	}

	t.Cleanup(func() {
		if closeErr := serverSession.Close(); closeErr != nil {
			t.Errorf("closing server session: %v", closeErr)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)

	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connecting client: %v", err)
	}

	t.Cleanup(func() {
		if closeErr := session.Close(); closeErr != nil {
			t.Errorf("closing client session: %v", closeErr)
		}
	})

	return session
}

// toolNames lists the tools a server exposes.
func toolNames(t *testing.T, srv *mcp.Server) []string {
	t.Helper()

	result, err := connect(t, srv).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}

	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}

	slices.Sort(names)

	return names
}

func TestNewAdvertisesServerIdentity(t *testing.T) {
	t.Parallel()

	info := connect(t, New(newGerrit(t), false)).InitializeResult().ServerInfo

	if info.Name != ServerName {
		t.Errorf("server name = %q, want %q", info.Name, ServerName)
	}

	if info.Version != version.Version {
		t.Errorf("server version = %q, want %q", info.Version, version.Version)
	}
}

func TestNewServesToolListing(t *testing.T) {
	t.Parallel()

	// What the set contains is pinned in inventory_test.go; what matters here
	// is that tools/list is answered at all.
	if _, err := connect(t, New(newGerrit(t), false)).ListTools(t.Context(), nil); err != nil {
		t.Fatalf("listing tools: %v", err)
	}
}

func TestReadOnlyServerOmitsExactlyTheWriteTools(t *testing.T) {
	t.Parallel()

	client := newGerrit(t)
	readOnly := toolNames(t, New(client, false))
	writable := toolNames(t, New(client, true))

	// Everything a read-only server exposes must also be on a writable one.
	for _, name := range readOnly {
		if !slices.Contains(writable, name) {
			t.Errorf("tool %q is missing once writes are allowed", name)
		}
	}

	// And the difference has to be accounted for by the write registry, not by
	// a read tool that quietly went missing.
	extra := 0

	for _, name := range writable {
		if !slices.Contains(readOnly, name) {
			extra++
		}
	}

	if extra != len(writeTools()) {
		t.Errorf("writable server adds %d tools, want %d from writeTools", extra, len(writeTools()))
	}

	if len(readOnly) != len(readTools()) {
		t.Errorf("read-only server exposes %d tools, want %d from readTools", len(readOnly), len(readTools()))
	}
}

func TestTextCarriesASingleTextContent(t *testing.T) {
	t.Parallel()

	result := text("42 changes")

	if result.IsError {
		t.Error("IsError = true, want false for a successful result")
	}

	if len(result.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(result.Content))
	}

	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] is %T, want *mcp.TextContent", result.Content[0])
	}

	if want := "42 changes"; content.Text != want {
		t.Errorf("Text = %q, want %q", content.Text, want)
	}
}
