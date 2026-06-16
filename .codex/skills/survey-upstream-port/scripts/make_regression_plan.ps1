param(
    [Parameter(Mandatory = $true)]
    [string[]]$Paths
)

$tests = New-Object System.Collections.Generic.HashSet[string]

foreach ($path in $Paths) {
    if ($path -match 'internal/providers/wjx') {
        $tests.Add('go test ./internal/providers/wjx ./tests') | Out-Null
    }
    if ($path -match 'internal/providers/tencent') {
        $tests.Add('go test ./internal/providers/tencent ./tests') | Out-Null
    }
    if ($path -match 'internal/providers/credamo') {
        $tests.Add('go test ./internal/providers/credamo ./tests') | Out-Null
    }
    if ($path -match 'internal/config|internal/models|internal/execution') {
        $tests.Add('go test ./internal/config ./internal/models ./tests') | Out-Null
    }
    if ($path -match 'internal/api|docs/sdk|README.md') {
        $tests.Add('go test ./internal/api ./tests') | Out-Null
    }
    if ($path -match 'internal/network|internal/engine|internal/tasks|internal/logging') {
        $tests.Add('go test ./...') | Out-Null
    }
}

$tests | Sort-Object | ForEach-Object { $_ }
