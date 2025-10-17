# Components

templ Components are markup and code that is compiled into functions that return a `templ.Component` interface by running the `templ generate` command.

Components can contain templ elements that render HTML, text, expressions that output text or include other templates, and branching statements such as `if` and `switch`, and `for` loops.

```templ title="header.templ"
package main

templ headerTemplate(name string) {
  <header data-testid="headerTemplate">
    <h1>{ name }</h1>
  </header>
}
```

The generated code is a Go function that returns a `templ.Component`.

```go title="header_templ.go"
func headerTemplate(name string) templ.Component {
  // Generated contents
}
```

`templ.Component` is an interface that has a `Render` method on it that is used to render the component to an `io.Writer`.

```go
type Component interface {
	Render(ctx context.Context, w io.Writer) error
}
```

:::tip
Since templ produces Go code, you can share templates the same way that you share Go code - by sharing your Go module.

templ follows the same rules as Go. If a `templ` block starts with an uppercase letter, then it is public, otherwise, it is private.

A `templ.Component` may write partial output to the `io.Writer` if it returns an error. If you want to ensure you only get complete output or nothing, write to a buffer first and then write the buffer to an `io.Writer`.
:::

## Code-only components

Since templ Components ultimately implement the `templ.Component` interface, any code that implements the interface can be used in place of a templ component generated from a `*.templ` file.

```go
package main

import (
	"context"
	"io"
	"os"

	"github.com/a-h/templ"
)

func button(text string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<button>"+text+"</button>")
		return err
	})
}

func main() {
	button("Click me").Render(context.Background(), os.Stdout)
}
```

```html title="Output"
<button>
 Click me
</button>
```

:::warning
This code is unsafe! In code-only components, you're responsible for escaping the HTML content yourself, e.g. with the `templ.EscapeString` function.
:::

## Method components

templ components can be returned from methods (functions attached to types).

Go code:

```templ
package main

import "os"

type Data struct {
	message string
}

templ (d Data) Method() {
	<div>{ d.message }</div>
}

func main() {
	d := Data{
		message: "You can implement methods on a type.",
	}
	d.Method().Render(context.Background(), os.Stdout)
}
```

It is also possible to initialize a struct and call its component method inline.

```templ
package main

import "os"

type Data struct {
	message string
}

templ (d Data) Method() {
	<div>{ d.message }</div>
}

templ Message() {
    <div>
        @Data{
            message: "You can implement methods on a type.",
        }.Method()
    </div>
}

func main() {
	Message().Render(context.Background(), os.Stdout)
}
```
# Template generation

To generate Go code from `*.templ` files, use the `templ` command line tool.

```
templ generate
```

The `templ generate` recurses into subdirectories and generates Go code for each `*.templ` file it finds.

The command outputs warnings, and a summary of updates.

```
(!) void element <input> should not have child content [ from=12:2 to=12:7 ]
(✓) Complete [ updates=62 duration=144.677334ms ]
```

## Advanced options

The `templ generate` command has a `--help` option that prints advanced options.

These include the ability to generate code for a single file and to choose the number of parallel workers that `templ generate` uses to create Go files.

By default `templ generate` uses the number of CPUs that your machine has installed.

```
templ generate --help
```

```
usage: templ generate [<args>...]

Generates Go code from templ files.

Args:
  -path <path>
    Generates code for all files in path. (default .)
  -f <file>
    Optionally generates code for a single file, e.g. -f header.templ
  -stdout
    Prints to stdout instead of writing generated files to the filesystem.
    Only applicable when -f is used.
  -source-map-visualisations
    Set to true to generate HTML files to visualise the templ code and its corresponding Go code.
  -include-version
    Set to false to skip inclusion of the templ version in the generated code. (default true)
  -include-timestamp
    Set to true to include the current time in the generated code.
  -watch
    Set to true to watch the path for changes and regenerate code.
  -cmd <cmd>
    Set the command to run after generating code.
  -proxy
    Set the URL to proxy after generating code and executing the command.
  -proxyport
    The port the proxy will listen on. (default 7331)
  -proxybind
    The address the proxy will listen on. (default 127.0.0.1)
  -notify-proxy
    If present, the command will issue a reload event to the proxy 127.0.0.1:7331, or use proxyport and proxybind to specify a different address.
  -w
    Number of workers to use when generating code. (default runtime.NumCPUs)
  -lazy
    Only generate .go files if the source .templ file is newer.	
  -pprof
    Port to run the pprof server on.
  -keep-orphaned-files
    Keeps orphaned generated templ files. (default false)
  -v
    Set log verbosity level to "debug". (default "info")
  -log-level
    Set log verbosity level. (default "info", options: "debug", "info", "warn", "error")
  -help
    Print help and exit.

Examples:

  Generate code for all files in the current directory and subdirectories:

    templ generate

  Generate code for a single file:

    templ generate -f header.templ

  Watch the current directory and subdirectories for changes and regenerate code:

    templ generate -watch
```

:::tip
The `templ generate --watch` option watches files for changes and runs templ generate when required.

However, the code generated in this mode is not optimised for production use.
:::

# Testing

To test that data is rendered as expected, there are two main ways to do it:

* Expectation testing - testing that specific expectations are met by the output.
* Snapshot testing - testing that outputs match a pre-written output.

## Expectation testing

Expectation testing validates that the right data appears in the output in the right format and position.

The example at https://github.com/a-h/templ/blob/main/examples/blog/posts_test.go shows how to test that a list of posts is rendered correctly.

These tests use the `goquery` library to parse HTML and check that expected elements are present. `goquery` is a jQuery-like library for Go, that is useful for parsing and querying HTML. You’ll need to run `go get github.com/PuerkitoBio/goquery` to add it to your `go.mod` file.

### Testing components

The test sets up a pipe to write templ's HTML output to, and reads the output from the pipe, parsing it with `goquery`.

First, we test the page header. To use `goquery` to inspect the output, we’ll need to connect the header component’s `Render` method to the `goquery.NewDocumentFromReader` function with an `io.Pipe`.

```go
func TestHeader(t *testing.T) {
    // Pipe the rendered template into goquery.
    r, w := io.Pipe()
    go func () {
        _ = headerTemplate("Posts").Render(context.Background(), w)
        _ = w.Close()
    }()
    doc, err := goquery.NewDocumentFromReader(r)
    if err != nil {
        t.Fatalf("failed to read template: %v", err)
    }
    // Expect the component to be present.
    if doc.Find(`[data-testid="headerTemplate"]`).Length() == 0 {
        t.Error("expected data-testid attribute to be rendered, but it wasn't")
    }
    // Expect the page name to be set correctly.
    expectedPageName := "Posts"
    if actualPageName := doc.Find("h1").Text(); actualPageName != expectedPageName {
        t.Errorf("expected page name %q, got %q", expectedPageName, actualPageName)
    }
}
```

The header template (the "subject under test") includes a placeholder for the page name, and a `data-testid` attribute that makes it easier to locate the `headerTemplate` within the HTML using a CSS selector of `[data-testid="headerTemplate"]`.

```go
templ headerTemplate(name string) {
    <header data-testid="headerTemplate">
        <h1>{ name }</h1>
    </header>
}
```

We can also test that the navigation bar was rendered.

```go
func TestNav(t *testing.T) {
    r, w := io.Pipe()
    go func() {
        _ = navTemplate().Render(context.Background(), w)
        _ = w.Close()
    }()
    doc, err := goquery.NewDocumentFromReader(r)
    if err != nil {
        t.Fatalf("failed to read template: %v", err)
    }
    // Expect the component to include a testid.
    if doc.Find(`[data-testid="navTemplate"]`).Length() == 0 {
        t.Error("expected data-testid attribute to be rendered, but it wasn't")
    }
}
```

Testing that it was rendered is useful, but it's even better to test that the navigation includes the correct `nav` items.

In this test, we find all of the `a` elements within the `nav` element, and check that they match the expected items.

```go
navItems := []string{"Home", "Posts"}

doc.Find("nav a").Each(func(i int, s *goquery.Selection) {
    expected := navItems[i]
    if actual := s.Text(); actual != expected {
        t.Errorf("expected nav item %q, got %q", expected, actual)
    }
})
```

To test the posts, we can use the same approach. We test that the posts are rendered correctly, and that the expected data is present.

### Testing whole pages

Next, we may want to go a level higher and test the entire page. 

Pages are also templ components, so the tests are structured in the same way.

There’s no need to test for the specifics about what gets rendered in the `navTemplate` or `homeTemplate` at the page level, because they’re already covered in other tests.

Some developers prefer to only test the external facing part of their code (e.g. at a page level), rather than testing each individual component, on the basis that it’s slower to make changes if the implementation is too tightly controlled.

For example, if a component is reused across pages, then it makes sense to test that in detail in its own test. In the pages or higher-order components that use it, there’s no point testing it again at that level, so we only check that it was rendered to the output by looking for its data-testid attribute, unless we also need to check what we're passing to it.

### Testing the HTTP handler

Finally, we want to test the posts HTTP handler. This requires a different approach.

We can use the `httptest` package to create a test server, and use the `net/http` package to make a request to the server and check the response.

The tests configure the `GetPosts` function on the `PostsHandler` with a mock that returns a "database error", while the other returns a list of two posts. Here's what the `PostsHandler` looks like:

```go
type PostsHandler struct {
    Log      *log.Logger
    GetPosts func() ([]Post, error)
}
```

In the error case, the test asserts that the error message was displayed, while in the success case, it checks that the `postsTemplate` is present. It does not check that the posts have actually been rendered properly or that specific fields are visible, because that’s already tested at the component level.

Testing it again here would make the code resistant to refactoring and rework, but then again, we might have missed actually passing the posts we got back from the database to the posts template, so it’s a matter of risk appetite vs refactor resistance.

Note the switch to the table-driven testing format, a popular approach in Go for testing multiple scenarios with the same test code.
```go
func TestPostsHandler(t *testing.T) {
    tests := []struct {
        name           string
        postGetter     func() (posts []Post, err error)
        expectedStatus int
        assert         func(doc *goquery.Document)
    }{
        {
            name: "database errors result in a 500 error",
            postGetter: func() (posts []Post, err error) {
                return nil, errors.New("database error")
            },
            expectedStatus: http.StatusInternalServerError,
            assert: func(doc *goquery.Document) {
                expected := "failed to retrieve posts\n"
                if actual := doc.Text(); actual != expected {
                    t.Errorf("expected error message %q, got %q", expected, actual)
                }
            },
        },
        {
            name: "database success renders the posts",
            postGetter: func() (posts []Post, err error) {
                return []Post{
                    {Name: "Name1", Author: "Author1"},
                    {Name: "Name2", Author: "Author2"},
                }, nil
            },
            expectedStatus: http.StatusInternalServerError,
            assert: func(doc *goquery.Document) {
                if doc.Find(`[data-testid="postsTemplate"]`).Length() == 0 {
                    t.Error("expected posts to be rendered, but it wasn't")
                }
            },
        },
    }
    for _, test := range tests {
        // Arrange.
        w := httptest.NewRecorder()
        r := httptest.NewRequest(http.MethodGet, "/posts", nil)

        ph := NewPostsHandler()
        ph.Log = log.New(io.Discard, "", 0) // Suppress logging.
        ph.GetPosts = test.postGetter

        // Act.
        ph.ServeHTTP(w, r)
        doc, err := goquery.NewDocumentFromReader(w.Result().Body)
        if err != nil {
            t.Fatalf("failed to read template: %v", err)
        }

        // Assert.
        test.assert(doc)
    }
}
```

### Summary

- goquery can be used effectively with templ for writing component level tests.
- Adding `data-testid` attributes to your code simplifies the test expressions you need to write to find elements within the output and makes your tests less brittle.
- Testing can be split between the two concerns of template rendering, and HTTP handlers.

## Snapshot testing

Snapshot testing is a more broad check. It simply checks that the output hasn't changed since the last time you took a copy of the output.

It relies on manually checking the output to make sure it's correct, and then "locking it in" by using the snapshot.

templ uses this strategy to check for regressions in behaviour between releases, as per https://github.com/a-h/templ/blob/main/generator/test-html-comment/render_test.go

To make it easier to compare the output against the expected HTML, templ uses a HTML formatting library before executing the diff.

```go
package testcomment

import (
	_ "embed"
	"testing"

	"github.com/a-h/templ/generator/htmldiff"
)

//go:embed expected.html
var expected string

func Test(t *testing.T) {
	component := render("sample content")

	diff, err := htmldiff.Diff(component, expected)
	if err != nil {
		t.Fatal(err)
	}
	if diff != "" {
		t.Error(diff)
	}
}
```
# View models

With templ, you can pass any Go type into your template as parameters, and you can call arbitrary functions.

However, if the parameters of your template don't closely map to what you're displaying to users, you may find yourself calling a lot of functions within your templ files to reshape or adjust data, or to carry out complex repeated string interpolation or URL constructions.

This can make template rendering hard to test, because you need to set up complex data structures in the right way in order to render the HTML. If the template calls APIs or accesses databases from within the templates, it's even harder to test, because then testing your templates becomes an integration test.

A more reliable approach can be to create a "View model" that only contains the fields that you intend to display, and where the data structure closely matches the structure of the visual layout.

```go
package invitesget

type Handler struct {
  Invites *InviteService
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
  invites, err := h.Invites.Get(getUserIDFromContext(r.Context()))
  if err != nil {
     //TODO: Log error server side.
  }
  m := NewInviteComponentViewModel(invites, err)
  teamInviteComponent(m).Render(r.Context(), w)
}

func NewInviteComponentViewModel(invites []models.Invite, err error) (m InviteComponentViewModel) {
  m.InviteCount = len(invites)
  if err != nil {
    m.ErrorMessage = "Failed to load invites, please try again"
  }
  return m
}


type InviteComponentViewModel struct {
  InviteCount int
  ErrorMessage string
}

templ teamInviteComponent(model InviteComponentViewModel) {
	if model.InviteCount > 0 {
		<div>You have { fmt.Sprintf("%d", model.InviteCount) } pending invites</div>
	}
        if model.ErrorMessage != "" {
		<div class="error">{ model.ErrorMessage }</div>
        }
}
```
