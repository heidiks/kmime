# kmime
<p align="center">
  <img width="150" height="154" alt="image" src="https://github.com/user-attachments/assets/86f1fb5e-0b1d-4e61-bae0-2dd187182fd0" />
</p>

`kmime` is a command-line utility that creates a temporary, interactive pod in Kubernetes by cloning the specification of an existing pod. It's designed for debugging, running one-off tasks, or exploring a pod's environment without altering the original resource.

## Features

- **Clone Existing Pods**: Instantly create a debug pod by copying the configuration of a running pod.
- **Interactive Shell**: Get an interactive `-it` shell (`bash` by default) in the new pod.
- **Automatic Cleanup**: The temporary pod is automatically deleted when your session ends.
- **Custom Naming**: Add a prefix and/or suffix to the new pod's name for easy identification.
- **User Identification**: Automatically appends your sanitized git email or local hostname to the pod name.
- **Custom Labels**: Inject custom labels into the new pod.
- **Inject Environment Variables**: Pass custom environment variables from a local file.
- **JSON Logging**: Keeps a record of all created pods in a local `kmime_log.json` file.

## Installation

To build `kmime` from the source, you need to have Go installed.

```bash
git clone https://github.com/heidiks/kmime.git
cd kmime

# Build the binary
go build -o kmime .

# Move the binary to a directory in your PATH
mv kmime /usr/local/bin/
```

## Usage

The basic command structure is:

```
kmime [source-pod-name] -n [namespace] [flags]
```

### Examples

**1. Basic Cloning**

Clone a pod named `my-app-pod-xyz` in the `production` namespace to get an interactive `bash` shell.

```bash
kmime my-app-pod-xyz -n production
```

**2. Custom Naming**

Clone the pod and add a prefix and suffix to the new pod's name.

```bash
kmime my-app-pod-xyz -n production --prefix temp- --suffix -debug
```
*This will create a pod named something like `temp-my-app-pod-xyz-debug-user-1234`.*

**3. Adding Custom Labels**

Add one or more labels to the new pod. This is useful for targeting the pod with specific service selectors or for organizational purposes.

```bash
kmime my-app-pod-xyz -n production -l "app=temp-debug" -l "owner=my-team"
```

**4. Injecting Environment Variables from a File**

Create a file named `my.env`:
```
# my.env
API_KEY=secret-key-for-debug
LOG_LEVEL=debug
```

Now, inject these variables into the new pod:

```bash
kmime my-app-pod-xyz -n production --env-file ./my.env
```
*Inside the new pod, `$API_KEY` and `$LOG_LEVEL` will be available.*

**5. Full Example**

A command combining multiple options:

```bash
kmime my-app-pod-xyz -n production \
  --prefix=debug- \
  --suffix=-task-123 \
  -l "app=importer" \
  -l "temp=true" \
  --env-file importer.env 
```

## Logging

`kmime` automatically creates a `kmime_log.json` file in the directory where you run the command. This file logs the details of every pod created, including timestamps, names, user, and all parameters used.

Example log entry:
```json
[
  {
    "timestamp": "2025-07-11T19:00:00.000Z",
    "new_pod_name": "debug-my-app-pod-xyz-task-123-user-5678",
    "source_pod": "my-app-pod-xyz",
    "namespace": "production",
    "user": "user-name",
    "command": [
      "bash"
    ],
    "prefix": "debug-",
    "suffix": "-task-123",
    "labels": {
      "app": "importer",
      "temp": "true"
    },
    "env_file": "importer.env"
  }
]
```
