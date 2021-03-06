package main

import (
	"database/sql"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/kiasaki/steamy/steamy-api/data"
	"github.com/kiasaki/steamy/steamy-api/util"
	"golang.org/x/crypto/bcrypt"
)

func notFound(w http.ResponseWriter, r *http.Request) {
	SetNotFoundResponse(w)
	WriteEntity(w, J{"error": "Page not found"})
}

func V1ApiIndex(w http.ResponseWriter, r *http.Request) {
	SetOKResponse(w, J{
		"data": J{
			"version": "v1",
			"motd":    "Steamy API V1",
		},
	})
}

type TokensCreateRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func V1TokensCreate(w http.ResponseWriter, r *http.Request) {
	var createRequest TokensCreateRequest
	err := Bind(r, &createRequest)
	if err != nil {
		SetBadRequestResponse(w)
		WriteEntity(w, J{"error": "Error reading request entity"})
		return
	}

	// Fetch user
	user, err := data.UsersFetchOneByEmail(createRequest.Email)
	if err != nil {
		SetInternalServerErrorResponse(w, err)
		return
	} else if user == data.UserNotFound {
		SetBadRequestResponse(w)
		WriteEntity(w, J{"error": "Email or password entered is incorrect"})
		return
	}

	// Compare passwords
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(createRequest.Password))
	if err != nil {
		SetBadRequestResponse(w)
		WriteEntity(w, J{"error": "Email or password entered is incorrect"})
		return
	}

	// Password matches, let's give the requester a token :D
	// Valid 1 week
	authToken, err := createAuthToken(user.Id, 7*24)
	if err != nil {
		SetInternalServerErrorResponse(w, err)
		return
	}

	SetOKResponse(w, J{
		"data": J{"token": authToken},
	})
}

func V1CurrentUser(w http.ResponseWriter, r *http.Request) {
	user, err := currentUser(r)
	if err != nil {
		SetInternalServerErrorResponse(w, err)
		return
	}

	user.Password = ""
	SetOKResponse(w, J{"data": user})
}

func BuildsShowArtifact(w http.ResponseWriter, r *http.Request) {
	var buildId = PathString(r, "buildId")
	var buildArtifactPath = buildArtifactPath(buildId)

	if _, err := os.Stat(buildArtifactPath); os.IsNotExist(err) {
		SetNotFoundResponse(w)
	} else if err != nil {
		SetInternalServerErrorResponse(w, err)
	} else {
		http.ServeFile(w, r, buildArtifactPath)
	}
}

func BuildsCreate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		SetInternalServerErrorResponse(w, err)
		return
	}

	project, err := data.ProjectsFetchOneByTitle(r.PostFormValue("project"))
	if err == sql.ErrNoRows {
		SetNotFoundResponse(w)
		WriteEntity(w, J{"error": "Project not found"})
	} else if err != nil {
		SetInternalServerErrorResponse(w, err)
	}

	// Get file from request
	file, _, err := r.FormFile("artifact")
	if err != nil {
		SetBadRequestResponse(w)
		return
	}
	defer file.Close()

	// Decide on new build id
	var buildId = util.NewUUID().String()

	// Save artifact to disk
	var newFilePath = buildArtifactPath(buildId)
	newFile, err := os.OpenFile(newFilePath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		SetInternalServerErrorResponse(w, err)
		WriteEntity(w, J{"error": "Error saving build to disk"})
		return
	}
	defer newFile.Close()

	io.Copy(newFile, file)

	// Create build
	var build = &data.Build{
		Id:         buildId,
		Version:    r.PostFormValue("version"),
		ProjectId:  project.Id,
		RepoUrl:    r.PostFormValue("repo_url"),
		RepoName:   r.PostFormValue("repo_name"),
		RepoBranch: r.PostFormValue("repo_branch"),
		RepoCommit: r.PostFormValue("repo_commit"),
		Publisher:  r.PostFormValue("publisher"),
		Created:    time.Now(),
	}
	err = data.BuildsCreate(build)
	if err != nil {
		SetInternalServerErrorResponse(w, err)
		WriteEntity(w, J{"error": "Error saving build to database"})
		return
	}

	SetNoContentResponse(w)
}
