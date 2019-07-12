package inflect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Name_Camel(t *testing.T) {
	r := require.New(t)
	table := []struct {
		V string
		E string
	}{
		{V: "foo_bar", E: "FooBar"},
		{V: "widget", E: "Widget"},
		{V: "User", E: "User"},
		{V: "user_id", E: "UserID"},
		{V: "post", E: "Post"},
	}
	for _, tt := range table {
		r.Equal(tt.E, Name(tt.V).Camel())
	}
}

func Test_Name_ParamID(t *testing.T) {
	r := require.New(t)
	table := []struct {
		V string
		E string
	}{
		{V: "foo_bar", E: "foo_bar_id"},
		{V: "admin/widget", E: "admin_widget_id"},
		{V: "widget", E: "widget_id"},
		{V: "User", E: "user_id"},
		{V: "Movies", E: "movie_id"},
		{V: "movies", E: "movie_id"},
		{V: "Movie", E: "movie_id"},
		{V: "Post", E: "post_id"},
	}
	for _, tt := range table {
		r.Equal(tt.E, Name(tt.V).ParamID())
	}
}

func Test_Name_Title(t *testing.T) {
	r := require.New(t)
	table := []struct {
		V string
		E string
	}{
		{V: "foo_bar", E: "Foo Bar"},
		{V: "admin/widget", E: "Admin Widget"},
		{V: "admin/post", E: "Admin Post"},
		{V: "widget", E: "Widget"},
		{V: "post", E: "Post"},
	}
	for _, tt := range table {
		r.Equal(tt.E, Name(tt.V).Title())
	}
}

func Test_Name_Model(t *testing.T) {
	r := require.New(t)
	table := []struct {
		V string
		E string
	}{
		{V: "foo_bar", E: "FooBar"},
		{V: "admin/widget", E: "AdminWidget"},
		{V: "widget", E: "Widget"},
		{V: "widgets", E: "Widget"},
		{V: "status", E: "Status"},
		{V: "Statuses", E: "Status"},
		{V: "statuses", E: "Status"},
		{V: "People", E: "Person"},
		{V: "people", E: "Person"},
		{V: "post", E: "Post"},
		{V: "posts", E: "Post"},
		{V: "admin/posts", E: "AdminPost"},
	}
	for _, tt := range table {
		r.Equal(tt.E, Name(tt.V).Model())
	}
}

func Test_Name_Resource(t *testing.T) {
	table := []struct {
		V string
		E string
	}{
		{V: "Person", E: "People"},
		{V: "foo_bar", E: "FooBars"},
		{V: "admin/widget", E: "AdminWidgets"},
		{V: "widget", E: "Widgets"},
		{V: "widgets", E: "Widgets"},
		{V: "greatPerson", E: "GreatPeople"},
		{V: "great/person", E: "GreatPeople"},
		{V: "status", E: "Statuses"},
		{V: "Status", E: "Statuses"},
		{V: "Statuses", E: "Statuses"},
		{V: "statuses", E: "Statuses"},
		{V: "post", E: "Posts"},
		{V: "posts", E: "Posts"},
		{V: "POSTS", E: "Posts"},
		{V: "POST", E: "Posts"},
		{V: "admin/post", E: "AdminPosts"},
		{V: "admin/posts", E: "AdminPosts"},
	}
	for _, tt := range table {
		t.Run(tt.V, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.E, Name(tt.V).Resource())
		})
	}
}

func Test_Name_ModelPlural(t *testing.T) {
	r := require.New(t)
	table := []struct {
		V string
		E string
	}{
		{V: "foo_bar", E: "FooBars"},
		{V: "admin/widget", E: "AdminWidgets"},
		{V: "widget", E: "Widgets"},
		{V: "widgets", E: "Widgets"},
		{V: "status", E: "Statuses"},
		{V: "statuses", E: "Statuses"},
		{V: "people", E: "People"},
		{V: "person", E: "People"},
		{V: "People", E: "People"},
		{V: "Status", E: "Statuses"},
		{V: "Post", E: "Posts"},
		{V: "post", E: "Posts"},
		{V: "posts", E: "Posts"},
		{V: "admin/posts", E: "AdminPosts"},
	}

	for _, tt := range table {
		r.Equal(tt.E, Name(tt.V).ModelPlural())
	}
}

func Test_Name_File(t *testing.T) {
	table := []struct {
		V string
		E string
	}{
		{V: "foo_bar", E: "foo_bar"},
		{V: "admin/widget", E: "admin/widget"},
		{V: "widget", E: "widget"},
		{V: "widgets", E: "widgets"},
		{V: "User", E: "user"},
		{V: "admin/posts", E: "admin/posts"},
		{V: "AdminPosts", E: "admin_posts"},
		{V: "post", E: "post"},
		{V: "posts", E: "posts"},
	}
	for _, tt := range table {
		t.Run(tt.V, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.E, Name(tt.V).File())
		})
	}
}

func Test_Name_VarCaseSingular(t *testing.T) {
	r := require.New(t)
	table := []struct {
		V string
		E string
	}{
		{V: "foo_bar", E: "fooBar"},
		{V: "admin/widget", E: "adminWidget"},
		{V: "widget", E: "widget"},
		{V: "widgets", E: "widget"},
		{V: "User", E: "user"},
		{V: "FooBar", E: "fooBar"},
		{V: "status", E: "status"},
		{V: "statuses", E: "status"},
		{V: "Status", E: "status"},
		{V: "Statuses", E: "status"},
		{V: "admin/post", E: "adminPost"},
		{V: "post", E: "post"},
		{V: "posts", E: "post"},
	}
	for _, tt := range table {
		r.Equal(tt.E, Name(tt.V).VarCaseSingular())
	}
}

func Test_Name_VarCasePlural(t *testing.T) {
	r := require.New(t)
	table := []struct {
		V string
		E string
	}{
		{V: "foo_bar", E: "fooBars"},
		{V: "admin/widget", E: "adminWidgets"},
		{V: "widget", E: "widgets"},
		{V: "widgets", E: "widgets"},
		{V: "User", E: "users"},
		{V: "FooBar", E: "fooBars"},
		{V: "status", E: "statuses"},
		{V: "statuses", E: "statuses"},
		{V: "Status", E: "statuses"},
		{V: "Statuses", E: "statuses"},
		{V: "admin/post", E: "adminPosts"},
		{V: "post", E: "posts"},
		{V: "posts", E: "posts"},
	}
	for _, tt := range table {
		r.Equal(tt.E, Name(tt.V).VarCasePlural())
	}
}

func Test_Name_Package(t *testing.T) {
	gp := os.Getenv("GOPATH")
	r := require.New(t)
	table := []struct {
		V string
		E string
	}{
		{V: filepath.Join(gp, "src", "admin/widget"), E: "admin/widget"},
		{V: filepath.Join(gp, "admin/widget"), E: "admin/widget"},
		{V: "admin/widget", E: "admin/widget"},
		{V: filepath.Join(gp, "src", "admin/post"), E: "admin/post"},
		{V: filepath.Join(gp, "admin/post"), E: "admin/post"},
		{V: "admin/post", E: "admin/post"},
	}
	for _, tt := range table {
		r.Equal(tt.E, Name(tt.V).Package())
	}
}

func Test_Name_Char(t *testing.T) {
	r := require.New(t)

	n := Name("Foo")
	r.Equal("f", n.Char())
}
