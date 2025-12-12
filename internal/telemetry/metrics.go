package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SurveyResponsesTotal tracks total number of survey responses
	// Note: No survey_slug label to avoid cardinality explosion
	SurveyResponsesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "survey_responses_total",
			Help: "Total number of survey responses",
		},
		[]string{"source"}, // source: "web" or "atproto"
	)

	// HTTPRequestDuration tracks HTTP request duration
	// Note: Use route patterns (e.g., "/surveys/:slug") not actual paths to bound cardinality
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)

	// Consumer metrics

	// JetstreamRecordsProcessed tracks records processed by the consumer
	JetstreamRecordsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "survey_jetstream_records_processed_total",
			Help: "Total number of ATProto records processed from Jetstream",
		},
		[]string{"collection", "operation", "status"}, // status: "success" or "error"
	)

	// JetstreamCursorLag tracks time since last processed event
	JetstreamCursorLag = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "survey_jetstream_cursor_lag_seconds",
			Help: "Seconds since the last processed Jetstream event (0 = real-time)",
		},
	)

	// JetstreamProcessingDuration tracks time to process each message
	JetstreamProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "survey_jetstream_processing_duration_seconds",
			Help:    "Time to process a Jetstream message",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"collection", "operation"},
	)

	// JetstreamConnectionStatus tracks WebSocket connection state
	JetstreamConnectionStatus = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "survey_jetstream_connected",
			Help: "Whether the Jetstream WebSocket is connected (1=connected, 0=disconnected)",
		},
	)

	// JetstreamReconnects tracks reconnection attempts
	JetstreamReconnects = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "survey_jetstream_reconnects_total",
			Help: "Total number of Jetstream reconnection attempts",
		},
	)

	// Business metrics for ATProto records

	// SurveysIndexed tracks surveys indexed from ATProto
	SurveysIndexed = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "survey_atproto_surveys_indexed_total",
			Help: "Total number of surveys indexed from ATProto PDSes",
		},
	)

	// SurveyQuestionCount tracks distribution of questions per survey
	SurveyQuestionCount = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "survey_atproto_questions_per_survey",
			Help:    "Distribution of question count per indexed survey",
			Buckets: []float64{1, 2, 3, 5, 10, 20, 50},
		},
	)

	// SurveyQuestionTypes tracks question types being used
	SurveyQuestionTypes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "survey_atproto_question_types_total",
			Help: "Count of question types across all indexed surveys",
		},
		[]string{"type"}, // single, multi, text
	)

	// VotesIndexed tracks votes indexed from ATProto
	VotesIndexed = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "survey_atproto_votes_indexed_total",
			Help: "Total number of votes indexed from ATProto",
		},
	)

	// VotesPerSurvey tracks vote distribution (low cardinality - buckets only)
	VotesPerSurvey = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "survey_atproto_votes_per_survey",
			Help:    "Distribution of vote counts per survey at time of indexing",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
	)

	// ResultsPublished tracks finalized results records
	ResultsPublished = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "survey_atproto_results_published_total",
			Help: "Total number of finalized results records published",
		},
	)

	// Note: Removed UniqueVoters and UniqueSurveyAuthors gauges
	// These require periodic DB queries to populate - use SQL queries in dashboards instead
)

// RegisterMetrics registers all Prometheus metrics
// This is called during application startup
func RegisterMetrics() {
	// Metrics are auto-registered via promauto, but we keep this function
	// for consistency and future manual registration if needed
}
