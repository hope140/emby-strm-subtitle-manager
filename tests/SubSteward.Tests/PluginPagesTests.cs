using System.IO;
using System.Linq;
using System.Reflection;
using MediaBrowser.Model.Plugins;
using SubSteward.Plugin;
using SubStewardPlugin = SubSteward.Plugin.Plugin;
using Xunit;

namespace SubSteward.Tests;

public sealed class PluginPagesTests
{
    [Fact]
    public void Plugin_ExposesTheEmbeddedAdminPage()
    {
        Assert.True(typeof(IHasWebPages).IsAssignableFrom(typeof(SubStewardPlugin)));

        var pageResource = typeof(SubStewardPlugin).Assembly
            .GetManifestResourceNames()
            .SingleOrDefault(name => name == "SubSteward.Plugin.Web.configPage.html");

        Assert.NotNull(pageResource);
        Assert.Contains("SubSteward.Plugin.Web.subSteward.js", typeof(SubStewardPlugin).Assembly.GetManifestResourceNames());
    }

    [Fact]
    public void EmbeddedAdminPage_ContainsTheM1WorkflowAndClosedBoundaries()
    {
        using var stream = typeof(SubStewardPlugin).Assembly.GetManifestResourceStream("SubSteward.Plugin.Web.configPage.html");
        Assert.NotNull(stream);
        using var reader = new StreamReader(stream!);
        var page = reader.ReadToEnd();

        Assert.Contains("data-bindheader=\"true\"", page);
        Assert.Contains("data-controller=\"__plugin/SubStewardUI7.js\"", page);
        Assert.Contains("is=\"emby-scroller\"", page);
        Assert.DoesNotContain("id=\"subStewardPage\"", page);
        Assert.Contains("全局状态", page);
        Assert.Contains("手动处理", page);
        Assert.Contains("全局默认", page);
        Assert.Contains("媒体库覆盖", page);
        Assert.Contains("ss-has-selection", page);
        Assert.Contains("ss-library-selected", page);
        Assert.Contains("更换媒体库", page);
        Assert.Contains("不会自动扫描全库", page);
        Assert.Contains("多 Source STRM", page);
        Assert.DoesNotContain("role=\"listitem\"", page);
        Assert.Contains("<select class=\"ss-select\" id=\"globalTargetLanguage\"", page);
        Assert.Contains("<select class=\"ss-select\" id=\"globalSecondaryLanguage\"", page);
        Assert.Contains("<select class=\"ss-select\" id=\"globalFormatOrder\"", page);
        Assert.Contains("<select class=\"ss-select\" id=\"libraryTargetLanguage\"", page);
        Assert.Contains("<select class=\"ss-select\" id=\"librarySecondaryLanguage\"", page);
        Assert.Contains("<select class=\"ss-select\" id=\"libraryFormatOrder\"", page);
        Assert.DoesNotContain("id=\"globalTargetLanguage\" type=\"text\"", page);
        Assert.DoesNotContain("id=\"globalSecondaryLanguage\" type=\"text\"", page);
        Assert.DoesNotContain("id=\"globalFormatOrder\" type=\"text\"", page);
        Assert.DoesNotContain("id=\"libraryTargetLanguage\" type=\"text\"", page);
        Assert.DoesNotContain("id=\"librarySecondaryLanguage\" type=\"text\"", page);
        Assert.DoesNotContain("id=\"libraryFormatOrder\" type=\"text\"", page);
        Assert.Contains("简体中文", page);
        Assert.Contains("繁体中文", page);
        Assert.Contains("英语", page);
        Assert.Contains("日语", page);
        Assert.Contains("ASS → SSA → SRT（默认）", page);

        using var controllerStream = typeof(SubStewardPlugin).Assembly.GetManifestResourceStream("SubSteward.Plugin.Web.subSteward.js");
        Assert.NotNull(controllerStream);
        using var controllerReader = new StreamReader(controllerStream!);
        var controller = controllerReader.ReadToEnd();
        Assert.Contains("define([], function ()", controller);
        Assert.Contains("return function (view, params)", controller);
        Assert.Contains("/SubSteward/Items", controller);
        Assert.Contains("/SubSteward/Summary", controller);
        Assert.Contains("/SubSteward/Browse", controller);
        Assert.Contains("/SubSteward/Libraries", controller);
        Assert.Contains("/SubSteward/Subtitles/Search", controller);
        Assert.Contains("/SubSteward/Subtitles/Fetch", controller);
        Assert.Contains("/SubSteward/Subtitles/Align", controller);
        Assert.Contains("/SubSteward/Subtitles/Install", controller);
        Assert.Contains("字幕提前", controller);
        Assert.Contains("字幕延后", controller);
        Assert.Contains("下载并校验", controller);
        Assert.Contains("更换媒体", controller);
        Assert.Contains("撤销上次", controller);
        Assert.Contains("已有字幕深检", controller);
        Assert.Contains("normalizeLanguageChoice", controller);
        Assert.Contains("normalizeFormatOrderChoice", controller);
        Assert.Contains("旧值，请改选支持项", controller);
        Assert.DoesNotContain("尚未开放", controller);
        Assert.Contains("itemLibraryFilter", page);
        Assert.Contains("itemPageSize", page);
        Assert.Contains("itemPagination", page);
        Assert.Contains("workflowSteps", page);
        Assert.Contains("openBrowseNode", controller);
        Assert.Contains("Series", controller);
        Assert.Contains("Season", controller);
        Assert.Contains("Episode", controller);
        Assert.Contains("--ss-left-text: #242424", page);
        Assert.Contains("下载校验超过 60 秒", controller);
        Assert.Contains("HTTP 429", controller);
        Assert.Contains("fetchingIndex", controller);
        Assert.Contains("PageSize", controller);
        Assert.Contains("totalCount", controller);
        Assert.Contains("browseBreadcrumb", page);
        Assert.Contains("LikelyNonFullRelease", controller);
        Assert.Contains("候选下载/校验失败", controller);
        Assert.DoesNotContain("<script>", page);
    }
}
