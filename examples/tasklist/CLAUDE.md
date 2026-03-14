# examples/tasklist

A task list web app demonstrating the hyper library's built-in HTML codec with a typewriter-inspired design.

## Running

Start the server in a tmux session:

    tmux new-session -d -s tasklist 'go run .'
    # The server runs on :8080

Stop it:

    tmux kill-session -t tasklist

## Testing

Run httptest-based tests:

    go test -v ./...

## Validation

### With curl

Fetch the task list as HTML:

    curl -s http://localhost:8080/ -H 'Accept: text/html'

Fetch as JSON:

    curl -s http://localhost:8080/ -H 'Accept: application/json' | jq .

Create a task:

    curl -s -X POST http://localhost:8080/tasks -d 'title=Buy+milk&status=pending' -H 'Accept: application/json' | jq .

Toggle a task:

    curl -s -X POST http://localhost:8080/tasks/1/toggle -H 'Accept: application/json' | jq .

Delete a task:

    curl -s -X DELETE http://localhost:8080/tasks/1 -H 'Accept: application/json' | jq .

### With screenshots

Take a screenshot of the running app using agent-browser:

    mise use -g github:vercel-labs/agent-browser@latest
    agent-browser install
    agent-browser screenshot http://localhost:8080/ --output screenshot.png

## Background Processes

All background processes (server, watchers) should be managed with tmux:

    tmux new-session -d -s <name> '<command>'
    tmux list-sessions
    tmux kill-session -t <name>

Never use bare `&` or `nohup`. Always use tmux so processes can be inspected and cleanly stopped.
