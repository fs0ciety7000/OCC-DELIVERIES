package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
)

// Brute force protection of PocketBase's own auth endpoints, on every auth
// collection (users and _superusers), persistent (ratelimit.go):
//
//   - auth-with-password / auth-with-otp: per IP (middleware, before the body
//     is read or a hash computed) and per identity (hooks); a success resets
//     the identity and gives the IP attempt back (an office behind one IP
//     only consumes its budget with failures);
//   - request-password-reset / request-verification / request-otp /
//     request-email-change: per IP, and per account. For the two anonymous
//     e-mail requests an account over its limit gets the usual 204 without
//     e-mail (no account enumeration).
var (
	loginIPLimit = newRateLimiter("login-ip", 20, 15*time.Minute)
	loginIDLimit = newRateLimiter("login-id", 8, 15*time.Minute)
	mailIPLimit  = newRateLimiter("authmail-ip", 10, 15*time.Minute)
	mailIDLimit  = newRateLimiter("authmail-id", 3, 15*time.Minute)
)

// authNow is the clock of the sign-in limits (tests).
var authNow = time.Now

// authAction returns the action of a POST /api/collections/{c}/{action}.
func authAction(path string) string {
	rest, ok := strings.CutPrefix(path, "/api/collections/")
	if !ok {
		return ""
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" {
		return ""
	}
	return parts[1]
}

func authIPLimiter(action string) (rateLimiter, bool) {
	switch action {
	case "auth-with-password", "auth-with-otp":
		return loginIPLimit, true
	case "request-password-reset", "request-verification", "request-otp", "request-email-change":
		return mailIPLimit, true
	}
	return rateLimiter{}, false
}

func tooManyAttempts(e *core.RequestEvent, retry time.Duration) error {
	return tooManyRequests(e, retry, domain.TooManyAttemptsMessage(retry))
}

func bindAuthThrottle(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.Bind(&hook.Handler[*core.RequestEvent]{
			Id: "occAuthThrottle",
			Func: func(e *core.RequestEvent) error {
				if e.Request.Method != http.MethodPost {
					return e.Next()
				}
				l, ok := authIPLimiter(authAction(e.Request.URL.Path))
				if !ok {
					return e.Next()
				}
				if ok, retry := l.allow(e.App, clientIP(e), authNow()); !ok {
					return tooManyAttempts(e, retry)
				}
				return e.Next()
			},
		})
		return se.Next()
	})

	app.OnRecordAuthWithPasswordRequest().BindFunc(func(e *core.RecordAuthWithPasswordRequestEvent) error {
		id := e.Collection.Id + ":" + domain.NormalizeIdentity(e.Identity)
		if ok, retry := loginIDLimit.allow(e.App, id, authNow()); !ok {
			return tooManyAttempts(e.RequestEvent, retry)
		}
		if err := e.Next(); err != nil {
			return err
		}
		// right password (MFA may still be pending)
		loginIDLimit.reset(e.App, id)
		loginIPLimit.refund(e.App, clientIP(e.RequestEvent))
		return nil
	})

	app.OnRecordAuthWithOTPRequest().BindFunc(func(e *core.RecordAuthWithOTPRequestEvent) error {
		if e.Record == nil {
			return e.Next()
		}
		id := "otp:" + e.Record.Collection().Id + ":" + e.Record.Id
		if ok, retry := loginIDLimit.allow(e.App, id, authNow()); !ok {
			return tooManyAttempts(e.RequestEvent, retry)
		}
		if err := e.Next(); err != nil {
			return err
		}
		loginIDLimit.reset(e.App, id)
		loginIPLimit.refund(e.App, clientIP(e.RequestEvent))
		return nil
	})

	silentMail := func(e *core.RequestEvent, r *core.Record, kind string) bool {
		if r == nil {
			return true
		}
		ok, _ := mailIDLimit.allow(e.App, kind+":"+r.Collection().Id+":"+r.Id, authNow())
		return ok
	}
	app.OnRecordRequestPasswordResetRequest().BindFunc(func(e *core.RecordRequestPasswordResetRequestEvent) error {
		if !silentMail(e.RequestEvent, e.Record, "reset") {
			return e.NoContent(http.StatusNoContent)
		}
		return e.Next()
	})
	app.OnRecordRequestVerificationRequest().BindFunc(func(e *core.RecordRequestVerificationRequestEvent) error {
		if !silentMail(e.RequestEvent, e.Record, "verify") {
			return e.NoContent(http.StatusNoContent)
		}
		return e.Next()
	})
	app.OnRecordRequestEmailChangeRequest().BindFunc(func(e *core.RecordRequestEmailChangeRequestEvent) error {
		if e.Record != nil {
			if ok, retry := mailIDLimit.allow(e.App, "email-change:"+e.Record.Collection().Id+":"+e.Record.Id, authNow()); !ok {
				return tooManyAttempts(e.RequestEvent, retry)
			}
		}
		return e.Next()
	})
}
