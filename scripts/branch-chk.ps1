param(
    [string]$Root = (Join-Path $HOME 'src')
)

Get-ChildItem $Root -Directory | ForEach-Object {
    $repoPath = $_.FullName

    if (-not (Test-Path (Join-Path $repoPath ".git"))) {
        return
    }

    git -C $repoPath fetch --quiet 2>$null

    $currentBranch = (git -C $repoPath branch --show-current 2>$null).Trim()
    $detached = -not $currentBranch
    if ($detached) { $currentBranch = "(detached)" }

    $dirty = git -C $repoPath status --porcelain

    $ahead = 0
    $behind = 0
    $hasUpstream = $false
    $syncCheckFailed = $false

    if (-not $detached) {
        $upstream = (git -C $repoPath rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>$null).Trim()
        if ($LASTEXITCODE -eq 0 -and $upstream) {
            $hasUpstream = $true

            $aheadOutput = (git -C $repoPath rev-list --count "$upstream..HEAD" 2>$null).Trim()
            $aheadExitCode = $LASTEXITCODE
            $behindOutput = (git -C $repoPath rev-list --count "HEAD..$upstream" 2>$null).Trim()
            $behindExitCode = $LASTEXITCODE

            if ($aheadExitCode -eq 0 -and $behindExitCode -eq 0) {
                $ahead = [int]$aheadOutput
                $behind = [int]$behindOutput
            } else {
                $syncCheckFailed = $true
            }
        }
    }

    $flags = @()

    if ($dirty) { $flags += "DIRTY" }
    if ($detached) {
        $flags += "DETACHED"
    } elseif (-not $hasUpstream) {
        $flags += "NO_UPSTREAM"
    } elseif ($syncCheckFailed) {
        $flags += "SYNC_CHECK_FAILED"
    } else {
        if ($ahead -gt 0) { $flags += "AHEAD:$ahead" }
        if ($behind -gt 0) { $flags += "BEHIND:$behind" }
        if ($ahead -gt 0 -and $behind -gt 0) { $flags += "DIVERGED" }
    }

    if ($flags.Count -gt 0) {
        "$repoPath : $currentBranch [" + ($flags -join ", ") + "]"
    }
}
