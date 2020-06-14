package main

import (
	contracts "github.com/estafette/estafette-ci-contracts"
)

type PipelinesListResponse struct {
	Items      []*contracts.Pipeline `json:"items"`
	Pagination contracts.Pagination  `json:"pagination"`
}

type PipelineBuildsListResponse struct {
	Items      []*contracts.Build   `json:"items"`
	Pagination contracts.Pagination `json:"pagination"`
}

type PipelineReleasesListResponse struct {
	Items      []*contracts.Release `json:"items"`
	Pagination contracts.Pagination `json:"pagination"`
}