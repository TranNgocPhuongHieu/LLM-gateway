package controller

import (
	"net/http"
	"strconv"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/gin-gonic/gin"
)

// adminSummaryMaxSpanSeconds caps root-admin summary queries to 366 days.
const adminSummaryMaxSpanSeconds int64 = 366 * 24 * 60 * 60

// parseAdminSummaryRange validates the required start_timestamp and
// end_timestamp query parameters. On failure it writes a 400 response.
func parseAdminSummaryRange(c *gin.Context) (int64, int64, bool) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid start_timestamp"})
		return 0, 0, false
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid end_timestamp"})
		return 0, 0, false
	}
	if endTimestamp < startTimestamp {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "end_timestamp must not be before start_timestamp"})
		return 0, 0, false
	}
	if endTimestamp-startTimestamp > adminSummaryMaxSpanSeconds {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "time span cannot exceed 366 days"})
		return 0, 0, false
	}
	return startTimestamp, endTimestamp, true
}

// GetAdminSummary returns platform-wide top-up, signup and request totals.
// Root admin only; mounted at GET /api/data/admin_summary.
func GetAdminSummary(c *gin.Context) {
	startTimestamp, endTimestamp, ok := parseAdminSummaryRange(c)
	if !ok {
		return
	}
	summary, err := model.GetAdminSummary(startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

// GetAdminErrorSummary returns categorized counts of terminal error logs across
// all users. Raw error content is never returned. Root admin only; mounted at
// GET /api/log/error_summary.
func GetAdminErrorSummary(c *gin.Context) {
	startTimestamp, endTimestamp, ok := parseAdminSummaryRange(c)
	if !ok {
		return
	}
	summary, err := model.GetErrorSummary(0, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}
