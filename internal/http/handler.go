package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/nurpe/snowops-operations/internal/http/middleware"
	"github.com/nurpe/snowops-operations/internal/model"
	"github.com/nurpe/snowops-operations/internal/service"
)

type Handler struct {
	areas       *service.AreaService
	polygons    *service.PolygonService
	monitoring  *service.MonitoringService
	log         zerolog.Logger
}

func NewHandler(
	areas *service.AreaService,
	polygons *service.PolygonService,
	monitoring *service.MonitoringService,
	log zerolog.Logger,
) *Handler {
	return &Handler{
		areas:      areas,
		polygons:   polygons,
		monitoring: monitoring,
		log:        log,
	}
}

func (h *Handler) Register(r *gin.Engine, authMiddleware gin.HandlerFunc) {
	protected := r.Group("/")
	protected.Use(authMiddleware)

	protected.GET("/cleaning-areas", h.listAreas)
	protected.POST("/cleaning-areas", h.createArea)
	protected.GET("/cleaning-areas/:id", h.getArea)
	protected.PATCH("/cleaning-areas/:id", h.updateArea)
	protected.PATCH("/cleaning-areas/:id/geometry", h.updateAreaGeometry)
	protected.GET("/cleaning-areas/:id/deletion-info", h.getAreaDeletionInfo)
	protected.DELETE("/cleaning-areas/:id", h.deleteArea)
	protected.GET("/cleaning-areas/:id/access", h.listAreaAccess)
	protected.POST("/cleaning-areas/:id/access", h.grantAreaAccess)
	protected.DELETE("/cleaning-areas/:id/access/:contractorId", h.revokeAreaAccess)
	protected.GET("/cleaning-areas/:id/ticket-template", h.areaTicketTemplate)

	protected.GET("/polygons", h.listPolygons)
	protected.POST("/polygons", h.createPolygon)
	protected.GET("/polygons/:id", h.getPolygon)
	protected.PATCH("/polygons/:id", h.updatePolygon)
	protected.PATCH("/polygons/:id/geometry", h.updatePolygonGeometry)
	protected.DELETE("/polygons/:id", h.deletePolygon)
	protected.GET("/polygons/:id/access", h.listPolygonAccess)
	protected.POST("/polygons/:id/access", h.grantPolygonAccess)
	protected.DELETE("/polygons/:id/access/:contractorId", h.revokePolygonAccess)

	protected.GET("/polygons/:id/cameras", h.listCameras)
	protected.POST("/polygons/:id/cameras", h.createCamera)
	protected.PATCH("/polygons/:id/cameras/:cameraId", h.updateCamera)

	integrations := protected.Group("/integrations")
	integrations.POST("/polygons/:id/contains", h.polygonContains)
	integrations.GET("/cameras/:id/polygon", h.cameraPolygon)

	monitoring := protected.Group("/monitoring")
	monitoring.GET("/vehicles-live", h.vehiclesLive)
	monitoring.GET("/vehicles/:id/track", h.vehicleTrack)
}

func (h *Handler) listAreas(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	statuses, err := parseAreaStatusQuery(c.QueryArray("status"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	onlyActive := parseBoolQuery(c.Query("only_active"))

	areas, err := h.areas.List(
		c.Request.Context(),
		principal,
		service.ListAreasInput{
			Status:     statuses,
			OnlyActive: onlyActive,
		},
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(areas))
}

type createAreaRequest struct {
	Name                string  `json:"name"`
	Description         *string `json:"description"`
	Geometry            string  `json:"geometry"`
	City                *string `json:"city"`
	Status              *string `json:"status"`
	DefaultContractorID *string `json:"default_contractor_id"`
}

func (h *Handler) createArea(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	var req createAreaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	var status *model.CleaningAreaStatus
	if req.Status != nil && strings.TrimSpace(*req.Status) != "" {
		value := model.CleaningAreaStatus(strings.ToUpper(strings.TrimSpace(*req.Status)))
		if !isValidCleaningAreaStatus(value) {
			c.JSON(http.StatusBadRequest, errorResponse("invalid status"))
			return
		}
		status = &value
	}

	var contractorID *uuid.UUID
	if req.DefaultContractorID != nil && strings.TrimSpace(*req.DefaultContractorID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*req.DefaultContractorID))
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("invalid default_contractor_id"))
			return
		}
		contractorID = &parsed
	}

	city := "Petropavlovsk"
	if req.City != nil && strings.TrimSpace(*req.City) != "" {
		city = strings.TrimSpace(*req.City)
	}

	area, err := h.areas.Create(
		c.Request.Context(),
		principal,
		service.CreateAreaInput{
			Name:                req.Name,
			Description:         req.Description,
			GeometryGeoJSON:     req.Geometry,
			City:                city,
			Status:              status,
			DefaultContractorID: contractorID,
		},
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, successResponse(area))
}

func (h *Handler) getArea(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	areaID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid area id"))
		return
	}

	area, err := h.areas.Get(c.Request.Context(), principal, areaID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(area))
}

type nullableUUID struct {
	Set  bool
	UUID *uuid.UUID
}

func (n *nullableUUID) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.UUID = nil
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	id, err := uuid.Parse(strings.TrimSpace(str))
	if err != nil {
		return err
	}
	n.UUID = &id
	return nil
}

type updateAreaRequest struct {
	Name                *string       `json:"name"`
	Description         *string       `json:"description"`
	Status              *string       `json:"status"`
	DefaultContractorID *nullableUUID `json:"default_contractor_id"`
	IsActive            *bool         `json:"is_active"`
}

func (h *Handler) updateArea(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	areaID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid area id"))
		return
	}

	var req updateAreaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	var status *model.CleaningAreaStatus
	if req.Status != nil && strings.TrimSpace(*req.Status) != "" {
		value := model.CleaningAreaStatus(strings.ToUpper(strings.TrimSpace(*req.Status)))
		if !isValidCleaningAreaStatus(value) {
			c.JSON(http.StatusBadRequest, errorResponse("invalid status"))
			return
		}
		status = &value
	}

	var contractorPtr **uuid.UUID
	if req.DefaultContractorID != nil && req.DefaultContractorID.Set {
		contractorPtr = new(*uuid.UUID)
		*contractorPtr = req.DefaultContractorID.UUID
	}

	area, err := h.areas.UpdateMetadata(
		c.Request.Context(),
		principal,
		service.UpdateAreaInput{
			ID:                  areaID,
			Name:                req.Name,
			Description:         req.Description,
			Status:              status,
			DefaultContractorID: contractorPtr,
			IsActive:            req.IsActive,
		},
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(area))
}

type updateGeometryRequest struct {
	Geometry string `json:"geometry"`
}

func (h *Handler) updateAreaGeometry(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	areaID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid area id"))
		return
	}

	var req updateGeometryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	area, err := h.areas.UpdateGeometry(c.Request.Context(), principal, areaID, req.Geometry)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(area))
}

func (h *Handler) listAreaAccess(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	areaID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid area id"))
		return
	}

	entries, err := h.areas.ListAccess(c.Request.Context(), principal, areaID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(gin.H{"access": entries}))
}

type grantAreaAccessRequest struct {
	ContractorID string  `json:"contractor_id" binding:"required"`
	Source       *string `json:"source"`
}

func (h *Handler) grantAreaAccess(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	areaID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid area id"))
		return
	}

	var req grantAreaAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	contractorID, err := uuid.Parse(strings.TrimSpace(req.ContractorID))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid contractor_id"))
		return
	}

	source := ""
	if req.Source != nil {
		source = *req.Source
	}

	if err := h.areas.GrantAccess(c.Request.Context(), principal, areaID, contractorID, source); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, successResponse(gin.H{"granted": true}))
}

func (h *Handler) revokeAreaAccess(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	areaID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid area id"))
		return
	}

	contractorID, err := parseUUIDParam(c, "contractorId")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid contractor id"))
		return
	}

	if err := h.areas.RevokeAccess(c.Request.Context(), principal, areaID, contractorID); err != nil {
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) getAreaDeletionInfo(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	areaID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid area id"))
		return
	}

	info, err := h.areas.GetDeletionInfo(c.Request.Context(), principal, areaID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(gin.H{
		"area": gin.H{
			"id":   info.Area.ID,
			"name": info.Area.Name,
		},
		"dependencies": gin.H{
			"tickets_count":        info.Dependencies.TicketsCount,
			"trips_count":          info.Dependencies.TripsCount,
			"assignments_count":    info.Dependencies.AssignmentsCount,
			"appeals_count":        info.Dependencies.AppealsCount,
			"violations_count":     info.Dependencies.ViolationsCount,
			"access_records_count": info.Dependencies.AccessRecordsCount,
		},
		"will_be_deleted": gin.H{
			"tickets":        info.Dependencies.TicketsCount > 0,
			"trips":          info.Dependencies.TripsCount > 0,
			"assignments":    info.Dependencies.AssignmentsCount > 0,
			"appeals":        info.Dependencies.AppealsCount > 0,
			"violations":     info.Dependencies.ViolationsCount > 0,
			"access_records": info.Dependencies.AccessRecordsCount > 0,
		},
	}))
}

func (h *Handler) deleteArea(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	areaID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid area id"))
		return
	}

	// Проверяем параметр force для каскадного удаления
	force := parseBoolQuery(c.Query("force"))

	if err := h.areas.Delete(c.Request.Context(), principal, areaID, force); err != nil {
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) areaTicketTemplate(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	areaID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid area id"))
		return
	}

	template, err := h.areas.TicketTemplate(c.Request.Context(), principal, areaID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(gin.H{
		"area":        template.Area,
		"contractors": template.AccessibleContractors,
	}))
}

func (h *Handler) listPolygons(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	onlyActive := parseBoolQuery(c.Query("only_active"))
	polygons, err := h.polygons.List(
		c.Request.Context(),
		principal,
		service.ListPolygonsInput{
			OnlyActive: onlyActive,
		},
	)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(polygons))
}

type createPolygonRequest struct {
	Name           string     `json:"name"`
	Address        *string    `json:"address"`
	Geometry       string     `json:"geometry"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty"` // Опционально, для LANDFILL устанавливается автоматически
	IsActive       *bool      `json:"is_active"`
}

func (h *Handler) createPolygon(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	var req createPolygonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	polygon, err := h.polygons.Create(
		c.Request.Context(),
		principal,
		service.CreatePolygonInput{
			Name:           req.Name,
			Address:        req.Address,
			Geometry:       req.Geometry,
			OrganizationID: req.OrganizationID,
			IsActive:       req.IsActive,
		},
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, successResponse(polygon))
}

func (h *Handler) getPolygon(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	id, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid polygon id"))
		return
	}

	polygon, err := h.polygons.Get(c.Request.Context(), principal, id)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(polygon))
}

type nullableString struct {
	Set   bool
	Value *string
}

func (n *nullableString) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	value := str
	n.Value = &value
	return nil
}

type updatePolygonRequest struct {
	Name     *string         `json:"name"`
	Address  *nullableString `json:"address"`
	IsActive *bool           `json:"is_active"`
}

func (h *Handler) updatePolygon(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	id, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid polygon id"))
		return
	}

	var req updatePolygonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	var addressPtr **string
	if req.Address != nil && req.Address.Set {
		addressPtr = new(*string)
		if req.Address.Value != nil {
			value := strings.TrimSpace(*req.Address.Value)
			if value == "" {
				*addressPtr = nil
			} else {
				v := value
				*addressPtr = &v
			}
		} else {
			*addressPtr = nil
		}
	}

	polygon, err := h.polygons.UpdateMetadata(
		c.Request.Context(),
		principal,
		service.UpdatePolygonInput{
			ID:       id,
			Name:     req.Name,
			Address:  addressPtr,
			IsActive: req.IsActive,
		},
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(polygon))
}

func (h *Handler) updatePolygonGeometry(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	id, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid polygon id"))
		return
	}

	var req updateGeometryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	polygon, err := h.polygons.UpdateGeometry(c.Request.Context(), principal, id, req.Geometry)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(polygon))
}

type grantPolygonAccessRequest struct {
	ContractorID string  `json:"contractor_id" binding:"required"`
	Source       *string `json:"source"`
}

func (h *Handler) listPolygonAccess(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	polygonID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid polygon id"))
		return
	}

	entries, err := h.polygons.ListAccess(c.Request.Context(), principal, polygonID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(gin.H{"access": entries}))
}

func (h *Handler) grantPolygonAccess(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	polygonID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid polygon id"))
		return
	}

	var req grantPolygonAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	contractorID, err := uuid.Parse(strings.TrimSpace(req.ContractorID))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid contractor_id"))
		return
	}

	source := ""
	if req.Source != nil {
		source = *req.Source
	}

	if err := h.polygons.GrantAccess(c.Request.Context(), principal, polygonID, contractorID, source); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, successResponse(gin.H{"granted": true}))
}

func (h *Handler) deletePolygon(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	polygonID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid polygon id"))
		return
	}

	if err := h.polygons.Delete(c.Request.Context(), principal, polygonID); err != nil {
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) revokePolygonAccess(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	polygonID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid polygon id"))
		return
	}

	contractorID, err := parseUUIDParam(c, "contractorId")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid contractor id"))
		return
	}

	if err := h.polygons.RevokeAccess(c.Request.Context(), principal, polygonID, contractorID); err != nil {
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) listCameras(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	polygonID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid polygon id"))
		return
	}

	cameras, err := h.polygons.ListCameras(c.Request.Context(), principal, polygonID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(cameras))
}

type createCameraRequest struct {
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Location *string `json:"location"`
	IsActive *bool   `json:"is_active"`
}

func (h *Handler) createCamera(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	polygonID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid polygon id"))
		return
	}

	var req createCameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	cameraType := model.CameraType(strings.ToUpper(strings.TrimSpace(req.Type)))
	if cameraType != model.CameraTypeLPR && cameraType != model.CameraTypeVolume {
		c.JSON(http.StatusBadRequest, errorResponse("invalid camera type"))
		return
	}

	camera, err := h.polygons.CreateCamera(
		c.Request.Context(),
		principal,
		service.CreateCameraInput{
			PolygonID: polygonID,
			Type:      cameraType,
			Name:      req.Name,
			Location:  req.Location,
			IsActive:  req.IsActive,
		},
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, successResponse(camera))
}

type updateCameraRequest struct {
	Type     *string         `json:"type"`
	Name     *string         `json:"name"`
	Location *nullableString `json:"location"`
	IsActive *bool           `json:"is_active"`
}

func (h *Handler) updateCamera(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	cameraID, err := parseUUIDParam(c, "cameraId")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid camera id"))
		return
	}

	var req updateCameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	var cameraType *model.CameraType
	if req.Type != nil && strings.TrimSpace(*req.Type) != "" {
		value := model.CameraType(strings.ToUpper(strings.TrimSpace(*req.Type)))
		cameraType = &value
	}

	var locationPtr **string
	if req.Location != nil && req.Location.Set {
		locationPtr = new(*string)
		if req.Location.Value != nil {
			value := strings.TrimSpace(*req.Location.Value)
			if value == "" {
				*locationPtr = nil
			} else {
				v := value
				*locationPtr = &v
			}
		} else {
			*locationPtr = nil
		}
	}

	camera, err := h.polygons.UpdateCamera(
		c.Request.Context(),
		principal,
		service.UpdateCameraInput{
			ID:       cameraID,
			Type:     cameraType,
			Name:     req.Name,
			Location: locationPtr,
			IsActive: req.IsActive,
		},
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(camera))
}

type polygonContainsRequest struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lng"`
}

func (h *Handler) polygonContains(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	polygonID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid polygon id"))
		return
	}

	var req polygonContainsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
		return
	}

	contains, err := h.polygons.ContainsPoint(c.Request.Context(), principal, polygonID, req.Latitude, req.Longitude)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(gin.H{"inside": contains}))
}

func (h *Handler) cameraPolygon(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	cameraID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid camera id"))
		return
	}

	camera, polygon, err := h.polygons.ResolveCameraPolygon(c.Request.Context(), principal, cameraID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(gin.H{
		"camera":  camera,
		"polygon": polygon,
	}))
}

func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPermissionDenied):
		c.JSON(http.StatusForbidden, errorResponse(err.Error()))
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, errorResponse(err.Error()))
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
	case errors.Is(err, service.ErrConflict) || errors.Is(err, service.ErrAreaHasTickets) || errors.Is(err, service.ErrPolygonHasTrips):
		c.JSON(http.StatusConflict, errorResponse(err.Error()))
	default:
		h.log.Error().Err(err).Msg("handler error")
		c.JSON(http.StatusInternalServerError, errorResponse("internal error"))
	}
}

func parseAreaStatusQuery(raw []string) ([]model.CleaningAreaStatus, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	values := make([]model.CleaningAreaStatus, 0, len(raw))
	seen := map[model.CleaningAreaStatus]struct{}{}

	for _, entry := range raw {
		for _, part := range strings.Split(entry, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			value := model.CleaningAreaStatus(strings.ToUpper(part))
			if !isValidCleaningAreaStatus(value) {
				return nil, errors.New("invalid status filter")
			}
			if _, exists := seen[value]; !exists {
				values = append(values, value)
				seen[value] = struct{}{}
			}
		}
	}

	return values, nil
}

func isValidCleaningAreaStatus(status model.CleaningAreaStatus) bool {
	return status == model.CleaningAreaStatusActive || status == model.CleaningAreaStatusInactive
}

func parseBoolQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseUUIDParam(c *gin.Context, param string) (uuid.UUID, error) {
	raw := strings.TrimSpace(c.Param(param))
	return uuid.Parse(raw)
}

func successResponse(data interface{}) gin.H {
	return gin.H{
		"data": data,
	}
}

func errorResponse(message string) gin.H {
	return gin.H{
		"error": message,
	}
}

func (h *Handler) vehiclesLive(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	// Парсим bbox (опционально)
	var bbox *service.BBox
	if minLat := c.Query("min_lat"); minLat != "" {
		var err error
		var minLatF, minLonF, maxLatF, maxLonF float64
		if minLatF, err = parseFloatQuery(c, "min_lat"); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("invalid min_lat"))
			return
		}
		if minLonF, err = parseFloatQuery(c, "min_lon"); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("invalid min_lon"))
			return
		}
		if maxLatF, err = parseFloatQuery(c, "max_lat"); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("invalid max_lat"))
			return
		}
		if maxLonF, err = parseFloatQuery(c, "max_lon"); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("invalid max_lon"))
			return
		}
		bbox = &service.BBox{
			MinLat: minLatF,
			MinLon: minLonF,
			MaxLat: maxLatF,
			MaxLon: maxLonF,
		}
	}

	// Парсим contractor_id (опционально)
	var contractorID *uuid.UUID
	if contractorIDStr := c.Query("contractor_id"); contractorIDStr != "" {
		parsed, err := uuid.Parse(contractorIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("invalid contractor_id"))
			return
		}
		contractorID = &parsed
	}

	vehicles, err := h.monitoring.GetVehiclesLive(
		c.Request.Context(),
		principal,
		service.VehiclesLiveInput{
			BBox:         bbox,
			ContractorID: contractorID,
		},
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(gin.H{
		"timestamp": time.Now().Format(time.RFC3339),
		"vehicles":   vehicles,
	}))
}

func (h *Handler) vehicleTrack(c *gin.Context) {
	principal, ok := middleware.MustPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, errorResponse("missing principal"))
		return
	}

	vehicleID, err := parseUUIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("invalid vehicle id"))
		return
	}

	// Парсим временной диапазон
	from := time.Now().Add(-1 * time.Hour) // По умолчанию последний час
	to := time.Now()

	if fromStr := c.Query("from"); fromStr != "" {
		parsed, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("invalid from parameter (use RFC3339 format)"))
			return
		}
		from = parsed
	}

	if toStr := c.Query("to"); toStr != "" {
		parsed, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("invalid to parameter (use RFC3339 format)"))
			return
		}
		to = parsed
	}

	points, err := h.monitoring.GetVehicleTrack(
		c.Request.Context(),
		principal,
		vehicleID,
		service.VehicleTrackInput{
			From: from,
			To:   to,
		},
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, successResponse(gin.H{
		"vehicle_id": vehicleID.String(),
		"from":       from.Format(time.RFC3339),
		"to":         to.Format(time.RFC3339),
		"points":     points,
	}))
}

func parseFloatQuery(c *gin.Context, param string) (float64, error) {
	raw := strings.TrimSpace(c.Query(param))
	if raw == "" {
		return 0, errors.New("empty value")
	}
	var value float64
	_, err := fmt.Sscanf(raw, "%f", &value)
	return value, err
}
