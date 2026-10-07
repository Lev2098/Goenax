// Command echo is a runnable Goenax demo on Echo: a few endpoints declared once,
// served live, documented at /api/docs.
//
//	go run ./examples/echo
//	open http://localhost:8080/api/docs      (Swagger UI)
//	     http://localhost:8080/api/scalar    (Scalar)
//	     http://localhost:8080/api/redoc     (Redoc)
package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"reflect"
	"time"

	"github.com/labstack/echo/v4"

	goenax "github.com/Lev2098/Goenax"
	echoadapter "github.com/Lev2098/Goenax/adapter/echo"
)

type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type Session struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type User struct {
	ID    string `json:"id"    validate:"uuid"`
	Email string `json:"email" validate:"email"`
	Role  string `json:"role"  validate:"oneof=admin member viewer"`
}

type UserList struct {
	Items []User `json:"items"`
	Total int    `json:"total"`
}

// ListUsersParams is bound by the handler (c.Bind) AND documented by
// goenax.Params — rename a tag here and both change together.
type ListUsersParams struct {
	Limit  int    `query:"limit"  validate:"min=1,max=100"                doc:"page size (default 20)"`
	Offset int    `query:"offset" validate:"min=0"                        doc:"items to skip"`
	Role   string `query:"role"   validate:"omitempty,oneof=admin member viewer" doc:"filter by role"`
}

type UserParams struct {
	ID string `param:"id" validate:"uuid" doc:"user id"`
}

type UpdateUserRequest struct {
	Role *string `json:"role,omitempty" validate:"omitempty,oneof=admin member viewer"`
}

// APIError is the body of every error response (Info.ErrorType).
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var errNotFound = errors.New("not found")

func main() {
	addr := flag.String("addr", "localhost:8080", "listen address")
	flag.Parse()

	e := echo.New()
	e.HideBanner = true

	reg := goenax.New()
	api := echoadapter.New(e.Group("/api"), reg, "/api")

	api.POST("/auth/login", login, []goenax.Option{
		goenax.Summary("Exchange credentials for a session"),
		goenax.Description("Returns a bearer token valid for one hour."),
		goenax.Owner("#auth"), goenax.Tags("Auth"),
		goenax.Request[LoginRequest](),
		goenax.Response[Session](http.StatusOK),
		goenax.Error(http.StatusBadRequest, "validation", "malformed body"),
		goenax.Error(http.StatusUnauthorized, "invalid", "wrong email or password"),
	})

	users := api.Group("/users")
	users.GET("", listUsers, []goenax.Option{
		goenax.Summary("List users"),
		goenax.Description("Paginated, optionally filtered by role."),
		goenax.Owner("#core"), goenax.Tags("Users"), goenax.Secured(),
		goenax.Params[ListUsersParams](),
		goenax.Response[UserList](http.StatusOK),
	})
	users.GET("/:id", getUser, []goenax.Option{
		goenax.Summary("Get a user"), goenax.Description("Fetch one user by id."),
		goenax.Owner("#core"), goenax.Tags("Users"), goenax.Secured(),
		goenax.Params[UserParams](),
		goenax.Response[User](http.StatusOK),
		goenax.Error(http.StatusNotFound, "not_found", "no such user"),
	})
	users.PATCH("/:id", updateUser, []goenax.Option{
		goenax.Summary("Update a user"), goenax.Description("Change a user's role."),
		goenax.Owner("#core"), goenax.Tags("Users"), goenax.Secured(),
		goenax.Params[UserParams](),
		goenax.Request[UpdateUserRequest](),
		goenax.NoContent(http.StatusNoContent),
		goenax.Error(http.StatusNotFound, "not_found", "no such user"),
	})

	info := goenax.Info{
		Title:     "Goenax Demo API",
		Version:   "dev",
		ErrorType: reflect.TypeFor[APIError](),
	}
	// In production, guard these with auth middleware or don't mount them.
	api.Docs("/docs", goenax.Docs(reg, info))
	api.Docs("/scalar", goenax.Docs(reg, info, goenax.WithUI(goenax.Scalar)))
	api.Docs("/redoc", goenax.Docs(reg, info, goenax.WithUI(goenax.Redoc)))

	// The same checks you would run in CI, run at boot for the demo.
	if problems := reg.Validate(); len(problems) > 0 {
		log.Fatalf("contract problems: %v", problems)
	}

	if undocumented, unmounted := reg.Coverage(echoadapter.Routes(e)); len(undocumented)+len(unmounted) > 0 {
		log.Fatalf("coverage: undocumented=%v unmounted=%v", undocumented, unmounted)
	}

	log.Printf("docs: http://%s/api/docs", *addr)
	log.Fatal(e.Start(*addr))
}

var demoUser = User{ID: "3f1c0b4e-8d2a-4c5e-9b7f-1a2b3c4d5e6f", Email: "ada@example.com", Role: "admin"}

func login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil || req.Email == "" {
		return c.JSON(http.StatusBadRequest, APIError{Code: "validation", Message: "malformed body"})
	}

	return c.JSON(http.StatusOK, Session{Token: "demo-token", ExpiresAt: time.Now().Add(time.Hour).UTC()})
}

func listUsers(c echo.Context) error {
	p := ListUsersParams{Limit: 20}
	if err := c.Bind(&p); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Code: "validation", Message: err.Error()})
	}

	items := []User{}
	if p.Role == "" || p.Role == demoUser.Role {
		items = append(items, demoUser)
	}

	return c.JSON(http.StatusOK, UserList{Items: items, Total: len(items)})
}

func getUser(c echo.Context) error {
	u, err := findUser(c)
	if err != nil {
		return c.JSON(http.StatusNotFound, APIError{Code: "not_found", Message: "no such user"})
	}

	return c.JSON(http.StatusOK, u)
}

func updateUser(c echo.Context) error {
	if _, err := findUser(c); err != nil {
		return c.JSON(http.StatusNotFound, APIError{Code: "not_found", Message: "no such user"})
	}

	return c.NoContent(http.StatusNoContent)
}

func findUser(c echo.Context) (User, error) {
	var p UserParams
	if err := c.Bind(&p); err != nil || p.ID != demoUser.ID {
		return User{}, errNotFound
	}

	return demoUser, nil
}
