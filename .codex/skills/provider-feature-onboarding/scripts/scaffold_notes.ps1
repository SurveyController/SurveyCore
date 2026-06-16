param(
    [Parameter(Mandatory = $true)]
    [string]$ProviderName
)

$base = "internal/providers/$ProviderName"

@(
    "$base/provider.go",
    "$base/parser.go",
    "$base/answer_builder.go",
    "$base/submit.go",
    "$base/client.go",
    "$base/types.go",
    "$base/provider_test.go"
)
