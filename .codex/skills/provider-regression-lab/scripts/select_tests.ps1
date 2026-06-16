param(
    [Parameter(Mandatory = $true)]
    [string[]]$Paths
)

$cmds = New-Object System.Collections.Generic.HashSet[string]

foreach ($path in $Paths) {
    switch -Regex ($path) {
        'internal/providers/wjx/' { $cmds.Add('go test ./internal/providers/wjx ./tests') | Out-Null; continue }
        'internal/providers/tencent/' { $cmds.Add('go test ./internal/providers/tencent ./tests') | Out-Null; continue }
        'internal/providers/credamo/' { $cmds.Add('go test ./internal/providers/credamo ./tests') | Out-Null; continue }
        'internal/providers/providerutil/' { $cmds.Add('go test ./internal/providers/... ./tests') | Out-Null; continue }
        'internal/providers/common\.go' { $cmds.Add('go test ./tests') | Out-Null; continue }
        'internal/config/' { $cmds.Add('go test ./internal/config ./tests') | Out-Null; continue }
        'internal/api/' { $cmds.Add('go test ./internal/api ./tests') | Out-Null; continue }
        'internal/network/proxy/' { $cmds.Add('go test ./internal/network/proxy') | Out-Null; continue }
        'internal/engine/|internal/tasks/' { $cmds.Add('go test ./internal/engine ./internal/tasks ./tests') | Out-Null; continue }
        'internal/models/|internal/execution/' { $cmds.Add('go test ./...') | Out-Null; continue }
    }
}

$cmds | Sort-Object | ForEach-Object { $_ }
