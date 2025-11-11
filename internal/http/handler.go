package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/nurpe/snowops-operations/internal/http/middleware"
	"github.com/nurpe/snowops-operations/internal/model"
	"github.com/nurpe/snowops-operations/internal/service"
)

type Handler struct {
	areas    *service.AreaService
	polygons *service.PolygonService
	log      zerolog.Logger
}

func NewHandler(
	areas *service.AreaService,
	polygons *service.PolygonService,
	log zerolog.Logger,
) *Handler {
	return &Handler{
		areas:    areas,
		polygons: polygons,
		log:      log,
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

	protected.GET("/polygons", h.listPolygons)
	protected.POST("/polygons", h.createPolygon)
	protected.GET("/polygons/:id", h.getPolygon)
	protected.PATCH("/polygons/:id", h.updatePolygon)
	protected.PATCH("/polygons/:id/geometry", h.updatePolygonGeometry)

	protected.GET("/polygons/:id/cameras", h.listCameras)
	protected.POST("/polygons/:id/cameras", h.createCamera)
	protected.PATCH("/polygons/:id/cameras/:cameraId", h.updateCamera)
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
	Name     string  `json:"name"`
	Address  *string `json:"address"`
	Geometry string  `json:"geometry"`
	IsActive *bool   `json:"is_active"`
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
			Name:     req.Name,
			Address:  req.Address,
			Geometry: req.Geometry,
			IsActive: req.IsActive,
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

func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPermissionDenied):
		c.JSON(http.StatusForbidden, errorResponse(err.Error()))
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, errorResponse(err.Error()))
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, errorResponse(err.Error()))
	case errors.Is(err, service.ErrConflict):
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
