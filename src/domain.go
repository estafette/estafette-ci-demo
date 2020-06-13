package main

import (
	contracts "github.com/estafette/estafette-ci-contracts"
)

type PipelinesListResponse struct {
	Items      []*contracts.Pipeline `json:"items"`
	Pagination contracts.Pagination  `json:"pagination"`
}
