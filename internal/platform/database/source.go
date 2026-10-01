// Choosing where a command reads its connection string: a fixed one, or the SSM parameter the functions use.

package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Source is where a command reads its connection string.
type Source struct {
	parameters ParameterReader
	// name is the SSM parameter's name, or DATABASE_URL for a fixed connection string; errors name it.
	name string
}

// SourceFromEnv returns the source that exactly one of two variables names:
//   - DATABASE_URL: a fixed connection string, such as a local database's.
//   - DATABASE_URL_PARAMETER: the SSM parameter holding the connection string, as the functions read it. It's read
//     with the ambient AWS credentials, and again whenever a connection fails.
//
// getenv reads a variable, such as os.Getenv. Errors never include the connection string.
func SourceFromEnv(ctx context.Context, getenv func(string) string) (Source, error) {
	databaseURL, parameterName := getenv("DATABASE_URL"), getenv("DATABASE_URL_PARAMETER")
	switch {
	case databaseURL != "" && parameterName != "":
		return Source{}, errors.New("set DATABASE_URL or DATABASE_URL_PARAMETER, not both")
	case databaseURL != "":
		return Source{parameters: fixedParameter(databaseURL), name: "DATABASE_URL"}, nil
	case parameterName != "":
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return Source{}, fmt.Errorf("load AWS configuration to read DATABASE_URL_PARAMETER: %v", err)
		}
		return Source{parameters: ssm.NewFromConfig(cfg), name: parameterName}, nil
	default:
		return Source{}, errors.New("set DATABASE_URL, or DATABASE_URL_PARAMETER to read the connection string from SSM")
	}
}

// Open returns a DB that connects with the source's connection string, as New does.
func (s Source) Open(schemaVersion int64) *DB {
	return New(s.parameters, s.name, schemaVersion)
}

// fixedParameter stands in for an SSM parameter whose value is a connection string that never changes.
type fixedParameter string

func (p fixedParameter) GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String(string(p))}}, nil
}
