# n8n + Vue + Go Workflow Manager Sample

This repository contains a local sample application for a workflow-driven request pipeline:

- Vue 3 frontend for request dashboard, workflow links, and temporary workflow runs
- Go backend with SQLite persistence
- n8n running in a separate Docker service
- Solace running in a separate Docker service
- Workflow JSON for validating filenames and publishing success/error events

## Core flow

1. A client sends a POST request to the Go API with a filename.
2. The Go API stores the request in SQLite and optionally triggers a workflow in n8n.
3. The n8n workflow checks whether the filename begins with `a=`.
4. If valid, it sends a Solace message with a JSON success payload.
5. If invalid, it sends a Solace message with a JSON error payload.
6. The Vue UI shows each request, the linked n8n workflow, and allows a temporary cloned workflow to be edited and run for that request.

## Included items

- Vue front-end dashboard with request table and workflow actions
- Go API for request tracking and workflow execution
- SQLite schema for each request entry
- n8n workflow JSON file for validation workflow
- Docker Compose setup for all services

## Quick start

```bash
docker compose -f docker/docker-compose.yml up --build
```

Then open:

- Frontend: http://localhost:5173
- Backend API: http://localhost:8081
- n8n: http://localhost:5678
- Solace admin UI: http://localhost:8080

## n8n workflow requirement

The sample workflow in `workflows/filename-validator-workflow.json` does the following:

- receives a POST request
- validates `filename` begins with `a=`
- publishes JSON message to the Solace endpoint
- returns success or error information
- includes Python code nodes for validation logic

## Notes

This is intentionally a sample and demo-friendly setup. It can be adapted to your real workflow and Solace broker configuration.
