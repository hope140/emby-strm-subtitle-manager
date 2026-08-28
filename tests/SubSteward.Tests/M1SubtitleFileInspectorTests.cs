using System;
using System.IO;
using System.Text;
using SubSteward.Plugin.M1;
using Xunit;

namespace SubSteward.Tests;

public sealed class M1SubtitleFileInspectorTests
{
    [Fact]
    public void Inspect_ReadsValidSiblingSubtitleAndReturnsValidation()
    {
        var root = CreateTempDirectory();
        try
        {
            var anchor = Path.Combine(root, "movie.strm");
            var subtitle = Path.Combine(root, "movie.zh-CN.srt");
            File.WriteAllText(anchor, "https://example.invalid/movie", new UTF8Encoding(false));
            File.WriteAllText(subtitle, "1\n00:00:01,000 --> 00:00:02,000\n你好\n", new UTF8Encoding(false));

            var inspection = new M1SubtitleFileInspector().Inspect(anchor, subtitle, ".srt");

            Assert.True(inspection.IsInspectable);
            Assert.NotNull(inspection.Validation);
            Assert.Equal("PASS", inspection.Validation.Health);
            Assert.Equal("UTF-8", inspection.Validation.Encoding);
            Assert.True(inspection.Validation.HasHanCharacters);
        }
        finally
        {
            DeleteTempDirectory(root);
        }
    }

    [Fact]
    public void Inspect_RejectsSubtitleOutsideAnchorDirectory()
    {
        var root = CreateTempDirectory();
        try
        {
            var anchorDirectory = Path.Combine(root, "media");
            var otherDirectory = Path.Combine(root, "other");
            Directory.CreateDirectory(anchorDirectory);
            Directory.CreateDirectory(otherDirectory);
            var anchor = Path.Combine(anchorDirectory, "movie.strm");
            var subtitle = Path.Combine(otherDirectory, "movie.srt");
            File.WriteAllText(anchor, "https://example.invalid/movie", new UTF8Encoding(false));
            File.WriteAllText(subtitle, "1\n00:00:01,000 --> 00:00:02,000\n你好\n", new UTF8Encoding(false));

            var inspection = new M1SubtitleFileInspector().Inspect(anchor, subtitle, ".srt");

            Assert.False(inspection.IsInspectable);
            Assert.Null(inspection.Validation);
            Assert.Contains(inspection.Reasons, reason => reason.Contains("safe local sidecar", StringComparison.OrdinalIgnoreCase));
        }
        finally
        {
            DeleteTempDirectory(root);
        }
    }

    private static string CreateTempDirectory()
    {
        var path = Path.Combine(Path.GetTempPath(), "substeward-inspector-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(path);
        return path;
    }

    private static void DeleteTempDirectory(string path)
    {
        if (Directory.Exists(path))
        {
            Directory.Delete(path, true);
        }
    }
}
