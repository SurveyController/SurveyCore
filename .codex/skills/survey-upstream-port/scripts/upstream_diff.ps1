param(
    [Parameter(Mandatory = $true)]
    [string]$RepoPath,

    [Parameter(Mandatory = $true)]
    [string]$Spec
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $RepoPath)) {
    throw "RepoPath 不存在: $RepoPath"
}

$resolved = Resolve-Path -LiteralPath $RepoPath
$files = git -C $resolved diff-tree --no-commit-id --name-only -r $Spec
$interesting = $files | Where-Object {
    $_ -match 'provider|survey|question|parser|submit|network|config|proxy|task|engine'
}

[pscustomobject]@{
    Spec = $Spec
    ChangedFiles = @($files)
    InterestingFiles = @($interesting)
} | ConvertTo-Json -Depth 4
