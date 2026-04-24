package responses

type Meta struct {
	TotalItems   int64 `json:"totalItems"`
	ItemCount    int64 `json:"itemCount"`
	ItemsPerPage int64 `json:"itemsPerPage"`
	TotalPages   int64 `json:"totalPages"`
	CurrentPage  int64 `json:"currentPage"`
}

type ApiResponse struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Status  string `json:"status,omitempty"`
}

type ApiResponsePaginated struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Meta    Meta   `json:"meta"`
}

func NewApiResponse(message string, data any) ApiResponse {
	return ApiResponse{
		Message: message,
		Data:    data,
	}
}

func NewApiResponsePaginated(message string, data any, meta Meta) ApiResponsePaginated {
	return ApiResponsePaginated{
		Message: message,
		Data:    data,
		Meta:    meta,
	}
}
