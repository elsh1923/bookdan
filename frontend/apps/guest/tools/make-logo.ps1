Add-Type -AssemblyName System.Drawing

$size = 1024
$bmp = New-Object System.Drawing.Bitmap $size, $size
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.SmoothingMode = 'AntiAlias'
$g.TextRenderingHint = 'AntiAliasGridFit'

# Bright background: warm sunrise gradient from amber to terracotta to coral.
$rect = New-Object System.Drawing.Rectangle 0, 0, $size, $size
$grad = New-Object System.Drawing.Drawing2D.LinearGradientBrush $rect, ([System.Drawing.Color]::FromArgb(255, 250, 183, 64)), ([System.Drawing.Color]::FromArgb(255, 224, 90, 71)), 60
$g.FillRectangle($grad, $rect)

# Soft sun circle behind the mark.
$sun = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(70, 255, 255, 255))
$g.FillEllipse($sun, 170, 130, 684, 684)

# White rounded "bed" silhouette: the hotel symbol.
$white = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::White)
$g.FillRectangle($white, 230, 640, 564, 90)             # mattress
$g.FillRectangle($white, 230, 500, 96, 230)             # headboard
$g.FillEllipse($white, 340, 560, 150, 100)              # pillow
$g.FillRectangle($white, 230, 730, 564, 36)              # frame bottom
$g.FillRectangle($white, 734, 600, 60, 166)             # footboard

# The letter B on top, in deep navy, so the name reads at small sizes too.
$font = New-Object System.Drawing.Font 'Georgia', 250, ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
$navy = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(255, 11, 25, 44))
$format = New-Object System.Drawing.StringFormat
$format.Alignment = 'Center'
$format.LineAlignment = 'Center'
$g.DrawString('B', $font, $navy, (New-Object System.Drawing.RectangleF 0, 150, $size, 400), $format)

$out = Join-Path $PSScriptRoot '..\assets\logo-bright.png'
$bmp.Save((Resolve-Path (Split-Path $out -Parent)).Path + '\logo-bright.png', [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose(); $bmp.Dispose()
Write-Output 'saved logo-bright.png'
