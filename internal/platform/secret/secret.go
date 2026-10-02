// Package secret gives a command a secret it reads by name from its environment: the value itself, as locally, or
// the SSM SecureString parameter holding it, as on Lambda. A parameter is read on first use and kept until Forget,
// so a rotated value takes effect without restarting the function, and only a command that uses the secret reads it.
package secret

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// readTimeout bounds each read of the parameter.
const readTimeout = 5 * time.Second

// ParameterReader is the part of the SSM client that reads a parameter.
type ParameterReader interface {
	GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// Secret is one secret's value. Its methods are safe for concurrent use.
type Secret struct {
	// parameters reads parameterName; both are empty for a value given directly.
	parameters    ParameterReader
	parameterName string

	mu sync.Mutex
	// value is the secret, or empty until a parameter is first read and after Forget.
	value string
}

// FromEnv returns the secret that exactly one of two variables names: name, such as GITHUB_CLIENT_SECRET, holding
// the value itself, or name_PARAMETER, such as GITHUB_CLIENT_SECRET_PARAMETER, naming the SSM parameter that holds
// it, read with the ambient AWS credentials. It returns nil when neither is set. getenv reads a variable, such as
// os.Getenv. Errors never include the value.
func FromEnv(ctx context.Context, getenv func(string) string, name string) (*Secret, error) {
	value, parameterName := getenv(name), getenv(name+"_PARAMETER")
	switch {
	case value != "" && parameterName != "":
		return nil, fmt.Errorf("set %s or %s_PARAMETER, not both", name, name)
	case value != "":
		return &Secret{value: value}, nil
	case parameterName != "":
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("load AWS configuration to read %s_PARAMETER: %v", name, err)
		}
		return FromParameter(ssm.NewFromConfig(cfg), parameterName), nil
	default:
		return nil, nil
	}
}

// FromParameter returns the secret in the SSM parameter name, which parameters reads.
func FromParameter(parameters ParameterReader, name string) *Secret {
	return &Secret{parameters: parameters, parameterName: name}
}

// Value returns the secret, reading its parameter unless an earlier read is kept.
func (s *Secret) Value(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value != "" || s.parameters == nil {
		return s.value, nil
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	out, err := s.parameters.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(s.parameterName), WithDecryption: aws.Bool(true)})
	if err != nil {
		return "", fmt.Errorf("read secret parameter=%q: %v", s.parameterName, err)
	}
	value := aws.ToString(out.Parameter.Value)
	// The parameter module creates a parameter with a placeholder for an operator to replace.
	if value == "" || value == placeholder {
		return "", fmt.Errorf("read secret parameter=%q: it holds no secret yet; set it in Parameter Store", s.parameterName)
	}
	s.value = value
	return value, nil
}

// Forget drops a value read from a parameter, so the next Value reads it again, such as after the secret was
// refused. A value given directly stays.
func (s *Secret) Forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parameters != nil {
		s.value = ""
	}
}

// placeholder is the value infra-catalog's ssm-secret-parameter module gives a parameter until an operator sets it.
const placeholder = "REPLACE_IN_AWS_CONSOLE"
