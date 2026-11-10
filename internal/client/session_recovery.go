package client

import (
	"connectrpc.com/connect"
	"context"
	"github.com/skillhubhq/skillhub/gen/api/auth/v1/authv1connect"
	"github.com/skillhubhq/skillhub/gen/api/registry/v1/registryv1connect"
	"time"
)

// 身份与实例探测为只读；短暂断连时重试一次，写入及单次授权领取不自动重放。
func identityRetryInterceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			result, err := next(ctx, req)
			procedure := req.Spec().Procedure
			if connect.CodeOf(err) != connect.CodeUnavailable || (procedure != authv1connect.AuthServiceGetIdentityProcedure && procedure != registryv1connect.RegistryServiceGetRegistryProcedure) {
				return result, err
			}
			timer := time.NewTimer(250 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
			}
			return next(ctx, req)
		}
	})
}
