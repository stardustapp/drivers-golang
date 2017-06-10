import (
  "github.com/danopia/stardust/star-router/base"
  gojenkins "github.com/andreaskoch/golang-jenkins"
)

func CreateClient(ctx base.Context, input *CreateClientInput) (output *Client) {
  auth := &gojenkins.Auth{
    Username: CreateClientInput.Username,
    ApiToken: CreateClientInput.ApiToken,
  }
  jenkins := gojenkins.NewJenkins(auth, CreateClientInput.BaseUrl)

  return &Client{
    svc: client,
  }
}