package micropub

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockRouterConfiguration struct{ mock.Mock }

var _ RouterConfiguration = &mockRouterConfiguration{}

func (m *mockRouterConfiguration) MediaEndpoint() string {
	return m.Called().Get(0).(string)
}

func (m *mockRouterConfiguration) SyndicateTo() []Syndication {
	return m.Called().Get(0).([]Syndication)
}

func (m *mockRouterConfiguration) Channels() []Channel {
	return m.Called().Get(0).([]Channel)
}

func (m *mockRouterConfiguration) Categories() []string {
	return m.Called().Get(0).([]string)
}

func (m *mockRouterConfiguration) PostTypes() []PostType {
	return m.Called().Get(0).([]PostType)
}

type mockRouterImplementation struct{ mock.Mock }

var _ RouterImplementation = &mockRouterImplementation{}

func (m *mockRouterImplementation) HasScope(r *http.Request, scope string) bool {
	return m.Called(r, scope).Get(0).(bool)
}

func (m *mockRouterImplementation) UploadMedia(file multipart.File, header *multipart.FileHeader) (string, error) {
	args := m.Called(file, header)
	return args.Get(0).(string), args.Error(1)
}

func (m *mockRouterImplementation) Source(url string) (map[string]any, error) {
	args := m.Called(url)
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *mockRouterImplementation) Create(req *Request) (string, error) {
	args := m.Called(req)
	return args.Get(0).(string), args.Error(1)
}

func (m *mockRouterImplementation) Update(req *Request) (string, error) {
	args := m.Called(req)
	return args.Get(0).(string), args.Error(1)
}

func (m *mockRouterImplementation) Delete(url string) error {
	return m.Called(url).Error(0)
}

func (m *mockRouterImplementation) Undelete(url string) error {
	return m.Called(url).Error(0)
}

func TestRouterPost(t *testing.T) {
	t.Parallel()

	t.Run("Valid Request", func(t *testing.T) {
		for _, request := range validRequests {
			config := &mockRouterConfiguration{}
			impl := &mockRouterImplementation{}

			switch request.response.Action {
			case ActionCreate:
				impl.Mock.On("HasScope", mock.Anything, "create").Return(true)
				impl.Mock.On("Create", request.response).Return("https://example.org/1", nil)
			case ActionUpdate:
				impl.Mock.On("HasScope", mock.Anything, "update").Return(true)
				impl.Mock.On("Update", request.response).Return(request.response.URL, nil)
			case ActionDelete:
				impl.Mock.On("HasScope", mock.Anything, "delete").Return(true)
				impl.Mock.On("Delete", request.response.URL).Return(nil)
			case ActionUndelete:
				impl.Mock.On("HasScope", mock.Anything, "undelete").Return(true)
				impl.Mock.On("Undelete", request.response.URL).Return(nil)
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/micropub", bytes.NewReader([]byte(request.body)))
			r.Header.Set("Content-Type", request.contentType)

			router := NewRouter(impl, config)
			router.MicropubHandler(w, r)

			switch request.response.Action {
			case ActionCreate:
				assert.Equal(t, "https://example.org/1", w.Result().Header.Get("Location"))
				assert.Equal(t, http.StatusAccepted, w.Result().StatusCode)
			case ActionUpdate:
				assert.Equal(t, request.response.URL, w.Result().Header.Get("Location"))
				assert.Equal(t, http.StatusOK, w.Result().StatusCode)
			case ActionDelete:
				assert.Equal(t, http.StatusOK, w.Result().StatusCode)
			case ActionUndelete:
				assert.Equal(t, http.StatusOK, w.Result().StatusCode)
			}
		}
	})

	t.Run("Valid Request, No Scope Permission", func(t *testing.T) {
		for _, request := range validRequests {
			config := &mockRouterConfiguration{}
			impl := &mockRouterImplementation{}

			switch request.response.Action {
			case ActionCreate:
				impl.Mock.On("HasScope", mock.Anything, "create").Return(false)
			case ActionUpdate:
				impl.Mock.On("HasScope", mock.Anything, "update").Return(false)
			case ActionDelete:
				impl.Mock.On("HasScope", mock.Anything, "delete").Return(false)
			case ActionUndelete:
				impl.Mock.On("HasScope", mock.Anything, "undelete").Return(false)
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/micropub", bytes.NewReader([]byte(request.body)))
			r.Header.Set("Content-Type", request.contentType)

			router := NewRouter(impl, config)
			router.MicropubHandler(w, r)

			body, err := io.ReadAll(w.Result().Body)
			assert.NoError(t, err)

			assert.Equal(t, http.StatusForbidden, w.Result().StatusCode)
			assert.Equal(t, `{"error":"insufficient_scope","error_description":"Insufficient scope."}`+"\n", string(body))
		}
	})

	t.Run("Valid Request, Implementation Errored", func(t *testing.T) {
		for _, request := range validRequests {
			config := &mockRouterConfiguration{}
			impl := &mockRouterImplementation{}

			magicError := errors.New("magic error")

			switch request.response.Action {
			case ActionCreate:
				impl.Mock.On("HasScope", mock.Anything, "create").Return(true)
				impl.Mock.On("Create", request.response).Return("", magicError)
			case ActionUpdate:
				impl.Mock.On("HasScope", mock.Anything, "update").Return(true)
				impl.Mock.On("Update", request.response).Return("", magicError)
			case ActionDelete:
				impl.Mock.On("HasScope", mock.Anything, "delete").Return(true)
				impl.Mock.On("Delete", request.response.URL).Return(magicError)
			case ActionUndelete:
				impl.Mock.On("HasScope", mock.Anything, "undelete").Return(true)
				impl.Mock.On("Undelete", request.response.URL).Return(magicError)
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/micropub", bytes.NewReader([]byte(request.body)))
			r.Header.Set("Content-Type", request.contentType)

			router := NewRouter(impl, config)
			router.MicropubHandler(w, r)

			body, err := io.ReadAll(w.Result().Body)
			assert.NoError(t, err)

			assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
			assert.Contains(t, string(body), "server_error")
		}
	})

	t.Run("Valid Requests, Custom Errors", func(t *testing.T) {
		for _, testCase := range []struct {
			err    error
			status int
		}{
			{ErrBadRequest, http.StatusBadRequest},
			{ErrNotFound, http.StatusNotFound},
			{ErrNotImplemented, http.StatusNotImplemented},
			{errors.New("something else"), http.StatusInternalServerError},
		} {
			body := "h=entry&content=hello+world&category[]=foo&category[]=bar"
			request := &Request{
				Action:   ActionCreate,
				Type:     "h-entry",
				Commands: map[string][]any{},
				Properties: map[string][]any{
					"category": {"foo", "bar"},
					"content":  {"hello world"},
				},
			}

			config := &mockRouterConfiguration{}
			impl := &mockRouterImplementation{}
			impl.Mock.On("HasScope", mock.Anything, "create").Return(true)
			impl.Mock.On("Create", request).Return("", testCase.err)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/micropub", bytes.NewReader([]byte(body)))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			router := NewRouter(impl, config)
			router.MicropubHandler(w, r)
			assert.Equal(t, testCase.status, w.Result().StatusCode)
		}
	})

	t.Run("Invalid Requests", func(t *testing.T) {
		for _, request := range invalidRequests {
			config := &mockRouterConfiguration{}
			impl := &mockRouterImplementation{}

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/micropub", bytes.NewReader([]byte(request.body)))
			r.Header.Set("Content-Type", request.contentType)

			router := NewRouter(impl, config)
			router.MicropubHandler(w, r)

			body, err := io.ReadAll(w.Result().Body)
			assert.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
			assert.Contains(t, string(body), "invalid_request")
		}
	})

}
