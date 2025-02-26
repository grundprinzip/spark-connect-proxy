// Licensed to the Apache Software Foundation (ASF) under one or more
// contributor license agreements.  See the NOTICE file distributed with
// this work for additional information regarding copyright ownership.
// The ASF licenses this file to You under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance with
// the License.  You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package errors

import (
	"fmt"
	"io"
)
import "github.com/go-errors/errors"

type wrappedError struct {
	errorType error
	cause     *errors.Error
}

func (w *wrappedError) Unwrap() []error {
	return []error{w.errorType, w.cause}
}

func (w *wrappedError) Error() string {
	return fmt.Sprintf("%s", w)
}

// WithType wraps an error with a type that can later be checked using `errors.Is`
func WithType(err error, errType errorType) error {
	return &wrappedError{cause: errors.Wrap(err, 1), errorType: errType}
}

func WithString(err error, errMsg string) error {
	return &wrappedError{cause: errors.Wrap(err, 1), errorType: errors.New(errMsg)}
}

func WithStringf(err error, errMsg string, params ...any) error {
	return &wrappedError{cause: errors.Wrap(err, 1), errorType: fmt.Errorf(errMsg, params...)}
}

// Format formats the error, supporting both short forms (v, s, q) and verbose form (+v)
func (w *wrappedError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = io.WriteString(s, "[sparkerror] ")
			_, _ = io.WriteString(s, fmt.Sprintf("Error Type: %s\n", w.errorType.Error()))
			_, _ = io.WriteString(s, fmt.Sprintf("Error Cause: %s\n%s", w.cause.Err.Error(), w.cause.Stack()))
			return
		}
		fallthrough
	case 's':
		_, _ = io.WriteString(s, fmt.Sprintf("%s: %s", w.errorType, w.cause))
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", w.errorType.Error())
	}
}

type errorType error

var (
	ProxyError        errorType = errorType(errors.New("proxy error"))
	ProxySessionError errorType = errorType(errors.New("proxy session error"))
	ProxyInitError    errorType = errorType(errors.New("proxy init error"))
)
