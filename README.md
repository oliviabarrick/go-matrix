go-matrix is a library and commandline tool that implements interacting with a Matrix home server and sending encrypted and
non-encrypted messages. It also includes a slack2matrix gateway for rewriting slack webhooks to matrix messages.

# Installation

Either use the Docker image `justinbarrick/slack2matrix` or install the command with `go get github.com/justinbarrick/go-matrix/cmd/matrixctl`.

# Usage

Register an account:

```
matrixctl register matrix.org user password
```

Login to an existing account:

```
matrixctl login matrix.org user password
```

Logout of an account:

```
matrixctl logout
```

Logout all sessions for an account:

```
matrixctl logout -a
```

Join a channel:

```
matrixctl join !asnetahoesnuth:matrix.org
```

Send a plaintext message to a channel:

```
matrixctl msg '!asnetahoesnuth:matrix.org' 'hi!'
```

Send an encrypted message to a channel:

```
matrixctl msg -e '!asnetahoesnuth:matrix.org' 'hi!'
```

Start an slack webhooks service on port 8000:

```
matrixctl slack2webhook '!asnetahoesnuth:matrix.org'
```

You can then send a message through the gateway:

```
docker run --env SLACK_WEBHOOK_URL=http://172.17.0.1:8000 suhlig/slack-message hi
```

# Deploying webhook service

To deploy the slack2webhook service, login or register:

```
matrixctl login matrix.org user password
```

You should now have a configuration file at `~/.matrix/config.json`.

Deploy the docker image along with the configuration file.
