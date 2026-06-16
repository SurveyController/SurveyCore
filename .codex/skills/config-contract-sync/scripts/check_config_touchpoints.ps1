param(
    [Parameter(Mandatory = $true)]
    [string]$Field
)

$targets = @(
    'internal/models/config.go',
    'internal/config/config.go',
    'internal/execution/config.go',
    'configs/example.json',
    'docs/sdk/schemas.md',
    'internal/config/config_test.go',
    'internal/models/execution_test.go'
)

foreach ($target in $targets) {
    if (Test-Path $target) {
        $hit = Select-String -Path $target -Pattern $Field -SimpleMatch
        [pscustomobject]@{
            Path = $target
            Found = [bool]$hit
        }
    }
}
