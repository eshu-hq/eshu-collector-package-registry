# Troubleshoot

Start from the failure class on the work item; it's the main signal
the host passes through. The full class tables are in
[operate](operate.md).

## Troubleshooting

`invalid_config` right after an edit usually means an unknown key or a
`visibility` outside `unknown`, `public`, and `private`. Because
rejections don't quote your config, compare with
`config.example.yaml`. Repeated `metadata_too_large` warnings mean
the feed's document really has outgrown the cap, so narrow `packages`
or the limits rather than retrying. A run that's quietly `partial`
with `registry_not_found` warnings often means derived targets are
guessing scopes that don't exist upstream. And for anything where the
host only shows you the class, reproduce it locally to read the stderr
JSON line.
