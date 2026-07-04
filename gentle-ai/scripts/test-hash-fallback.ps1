# Test script to verify .NET SHA256 fallback works correctly
# This ensures the fallback path produces valid SHA256 hashes even when Get-FileHash is unavailable

$testFile = "$env:TEMP\gentle-ai-hash-test.txt"
# Use fixed content for deterministic testing
$testContent = "Gentle AI SHA256 fallback test"

try {
    # Create test file with UTF-8 encoding (no BOM)
    [System.IO.File]::WriteAllText($testFile, $testContent, [System.Text.UTF8Encoding]::new($false))

    # Calculate hash using .NET fallback (the path we're testing)
    $sha256 = [System.Security.Cryptography.SHA256]::Create()
    $fileStream = [System.IO.File]::OpenRead($testFile)
    try {
        $hashBytes = $sha256.ComputeHash($fileStream)
        $fallbackHash = [System.BitConverter]::ToString($hashBytes).Replace("-", "").ToLowerInvariant()
    } finally {
        $fileStream.Close()
        $sha256.Dispose()
    }

    # Verify the hash is valid (64 hex characters for SHA256)
    if ($fallbackHash -match '^[a-f0-9]{64}$') {
        Write-Host "PASS: .NET fallback produces valid SHA256 hash" -ForegroundColor Green
        Write-Host "Hash: $fallbackHash"
        
        # If Get-FileHash is available, verify both methods match
        if (Get-Command Get-FileHash -ErrorAction SilentlyContinue) {
            $standardHash = (Get-FileHash -Path $testFile -Algorithm SHA256).Hash.ToLowerInvariant()
            if ($standardHash -eq $fallbackHash) {
                Write-Host "PASS: .NET fallback matches Get-FileHash" -ForegroundColor Green
            } else {
                Write-Host "FAIL: Hash mismatch between Get-FileHash and .NET fallback" -ForegroundColor Red
                Write-Host "Get-FileHash: $standardHash"
                Write-Host ".NET fallback: $fallbackHash"
                exit 1
            }
        } else {
            Write-Host "INFO: Get-FileHash not available, skipping comparison test" -ForegroundColor Yellow
        }
        
        exit 0
    } else {
        Write-Host "FAIL: .NET fallback produced invalid hash format" -ForegroundColor Red
        Write-Host "Got: $fallbackHash"
        Write-Host "Expected: 64 hex characters"
        exit 1
    }
} finally {
    # Cleanup
    if (Test-Path $testFile) {
        Remove-Item -Path $testFile -Force
    }
}
