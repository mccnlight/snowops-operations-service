package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/nurpe/snowops-operations/internal/auth"
	"github.com/nurpe/snowops-operations/internal/model"
)

const (
	claimsContextKey    = "tokenClaims"
	principalContextKey = "principal"
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer"
)

func Auth(parser *auth.Parser) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawHeader := c.GetHeader(authorizationHeader)
		if rawHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header missing"})
			return
		}

		parts := strings.SplitN(rawHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
			return
		}

		claims, err := parser.Parse(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		principal := model.Principal{
			UserID:         claims.UserID,
			OrganizationID: claims.OrgID,
			Role:           claims.Role,
		}

		c.Set(claimsContextKey, claims)
		c.Set(principalContextKey, principal)
		c.Next()
	}
}

func MustPrincipal(c *gin.Context) (model.Principal, bool) {
	value, exists := c.Get(principalContextKey)
	if !exists {
		return model.Principal{}, false
	}

	principal, ok := value.(model.Principal)
	if !ok {
		return model.Principal{}, false
	}

	return principal, true
}
