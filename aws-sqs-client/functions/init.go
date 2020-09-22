package driver

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	//"github.com/aws/aws-sdk-go/service/sqs"
)

func InitImpl(input *Credentials) (output *Client) {
	creds := credentials.NewStaticCredentials(input.AccessKeyID, input.SecretAccessKey, "")

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region:      aws.String(input.Region),
			Credentials: creds,
		},
	}))

	return &Client{
		session: sess,
	}
}
