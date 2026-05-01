package planner

import "github.com/gin-gonic/gin"

// RegisterRoutes wires planner merchant and profile endpoints.
func RegisterRoutes(authorized, adminRoutes gin.IRoutes, merchantHandler *MerchantHandler, profileHandler *ProfileHandler) {
	authorized.GET("/planner/merchants", merchantHandler.ListMerchants)
	authorized.GET("/planner/merchants/:id", merchantHandler.GetMerchant)
	authorized.GET("/planner/profiles", profileHandler.ListProfiles)
	authorized.GET("/planner/profiles/me", profileHandler.GetMyProfile)
	authorized.PUT("/planner/profiles/me", profileHandler.UpdateMyProfile)
	authorized.GET("/planner/profiles/:id", profileHandler.GetProfile)

	adminRoutes.POST("/planner/merchants", merchantHandler.CreateMerchant)
	adminRoutes.PUT("/planner/merchants/:id", merchantHandler.UpdateMerchant)
	adminRoutes.POST("/planner/profiles", profileHandler.CreateProfile)
}
