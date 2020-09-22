package driver

import (
	gojenkins "github.com/andreaskoch/golang-jenkins"
)

func CreateClientImpl(input *CreateClientInput) (output *Client) {
	auth := &gojenkins.Auth{
		Username: input.Username,
		ApiToken: input.APIToken,
	}
	jenkins := gojenkins.NewJenkins(auth, input.BaseURL)

	return &Client{
		svc: jenkins,
	}
}
