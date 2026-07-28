// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package runner

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	actions_model "code.gitea.io/gitea/models/actions"
	auth_model "code.gitea.io/gitea/models/auth"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"

	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	uuidHeaderKey  = "x-runner-uuid"
	tokenHeaderKey = "x-runner-token"
)

var withRunner = connect.WithInterceptors(connect.UnaryInterceptorFunc(func(unaryFunc connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		methodName := getMethodNameFromProcedure(request.Spec().Procedure)
		if methodName == "Register" {
			return unaryFunc(ctx, request)
		}
		uuid := request.Header().Get(uuidHeaderKey)
		token := request.Header().Get(tokenHeaderKey)

		runner, err := actions_model.GetRunnerByUUID(ctx, uuid)
		if err != nil {
			if errors.Is(err, util.ErrNotExist) {
				return nil, status.Error(codes.Unauthenticated, "unregistered runner")
			}
			return nil, status.Error(codes.Internal, err.Error())
		}
		if subtle.ConstantTimeCompare([]byte(runner.TokenHash), []byte(auth_model.HashToken(token, runner.TokenSalt))) != 1 {
			return nil, status.Error(codes.Unauthenticated, "unregistered runner")
		}

		cols := []string{"last_online"}
		runner.LastOnline = timeutil.TimeStampNow()
		if methodName == "UpdateTask" || methodName == "UpdateLog" {
			runner.LastActive = timeutil.TimeStampNow()
			cols = append(cols, "last_active")
		}
		if err := actions_model.UpdateRunner(ctx, runner, cols...); err != nil {
			log.Error("can't update runner status: %v", err)
		}

		ctx = context.WithValue(ctx, runnerCtxKey{}, runner)
		return unaryFunc(ctx, request)
	}
}))

// getMethodNameFromProcedure extracts the method name from a Connect procedure,
// which always has the canonical gRPC form "/<package.Service>/<Method>".
// It returns an empty string if the procedure is malformed, e.g. when the
// service segment between the leading slash and the method name is empty.
func getMethodNameFromProcedure(procedure string) string {
	if !strings.HasPrefix(procedure, "/") {
		return ""
	}
	rest := procedure[1:]
	slash := strings.LastIndexByte(rest, '/')
	// require a non-empty service segment before the slash and a method name after it
	if slash <= 0 || slash == len(rest)-1 {
		return ""
	}
	return rest[slash+1:]
}

type runnerCtxKey struct{}

func GetRunner(ctx context.Context) *actions_model.ActionRunner {
	if v := ctx.Value(runnerCtxKey{}); v != nil {
		if r, ok := v.(*actions_model.ActionRunner); ok {
			return r
		}
	}
	return nil
}
