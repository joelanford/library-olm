package api

import (
	"encoding/json"
	"net/http"
)

const problemTypeBase = "https://library-olm.dev/problems/v1/"

type problemKind struct {
	typeName string
	title    string
	status   int
}

var (
	problemMalformedInput = problemKind{
		typeName: "malformed-input",
		title:    "Malformed input",
		status:   http.StatusBadRequest,
	}
	problemInvalidCatalogSelector = problemKind{
		typeName: "invalid-catalog-selector",
		title:    "Invalid catalog selector",
		status:   http.StatusBadRequest,
	}
	problemInvalidVersionConstraint = problemKind{
		typeName: "invalid-version-constraint",
		title:    "Invalid semantic-version constraint",
		status:   http.StatusBadRequest,
	}
	problemUnsupportedPolicy = problemKind{
		typeName: "unsupported-upgrade-constraint-policy",
		title:    "Unsupported upgrade constraint policy",
		status:   http.StatusBadRequest,
	}
	problemInvalidCursor = problemKind{
		typeName: "invalid-cursor",
		title:    "Invalid cursor",
		status:   http.StatusBadRequest,
	}
	problemNotFound = problemKind{
		typeName: "not-found",
		title:    "Resource not found",
		status:   http.StatusNotFound,
	}
	problemMethodNotAllowed = problemKind{
		typeName: "method-not-allowed",
		title:    "Method not allowed",
		status:   http.StatusMethodNotAllowed,
	}
	problemStaleCursor = problemKind{
		typeName: "stale-cursor",
		title:    "Stale cursor",
		status:   http.StatusConflict,
	}
	problemAmbiguousPackage = problemKind{
		typeName: "ambiguous-package",
		title:    "Ambiguous package",
		status:   http.StatusConflict,
	}
	problemUnsupportedMediaType = problemKind{
		typeName: "unsupported-media-type",
		title:    "Unsupported media type",
		status:   http.StatusUnsupportedMediaType,
	}
	problemCatalogReadFailure = problemKind{
		typeName: "catalog-read-failure",
		title:    "Catalog read failure",
		status:   http.StatusInternalServerError,
	}
)

type invalidParameter struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type problemDetails struct {
	Type          string             `json:"type"`
	Title         string             `json:"title"`
	Status        int                `json:"status"`
	Detail        string             `json:"detail,omitempty"`
	Instance      string             `json:"instance,omitempty"`
	InvalidParams []invalidParameter `json:"invalidParams,omitempty"`
}

func newProblem(kind problemKind, invalidParams ...invalidParameter) problemDetails {
	return problemDetails{
		Type:          problemTypeBase + kind.typeName,
		Title:         kind.title,
		Status:        kind.status,
		InvalidParams: invalidParams,
	}
}

func writeProblem(w http.ResponseWriter, r *http.Request, problem problemDetails) {
	problem.Instance = r.URL.RequestURI()
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}

func invalidParam(name, reason string) invalidParameter {
	return invalidParameter{Name: name, Reason: reason}
}
