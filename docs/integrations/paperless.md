# Paperless-ngx

> Version française : [../fr/integrations/paperless.md](../fr/integrations/paperless.md)

Document count, inbox, consumption tasks and storage. Polled every minute.

## Configuration

- Secret `token`: the user's API token (My Profile > API token). A read-only account is enough for statistics and tasks; `/api/status/` (version, storage) requires an administrator.
- Options: none.

```yaml
- id: paperless
  type: paperless
  title: Paperless
  url: http://paperless:8000
  secrets: {token: paperless_token}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| documents | « Documents » (documents) | tile |
| inbox | « Boîte de réception » (inbox; orange if not empty) | tile |
| tasks | « Tâches » (tasks): running, failures over 24 h | tile |
| storage | « Stockage » (storage) | detail |
| classes | tags / correspondents / types | detail |
| chars | indexed characters | detail |
| types | file types | detail |
| task_list | latest tasks | detail |
| version | « Version » (version) | detail |

## Limits

- `/api/tasks/`: an array or a `{results}` page depending on the version; both are read.
- Not proven on a real instance.
