using System;
using System.Collections.Generic;
using MediaBrowser.Common.Configuration;
using MediaBrowser.Common.Plugins;
using MediaBrowser.Model.Serialization;
using MediaBrowser.Model.Plugins;

namespace SubSteward.Plugin
{
    /// <summary>
    /// The stable Emby plugin identity and its server-managed data folder.
    /// </summary>
    public sealed class Plugin : BasePlugin<PluginConfiguration>, IHasWebPages
    {
        private static Plugin instance;

        public static Plugin Instance
        {
            get
            {
                if (instance == null)
                {
                    throw new InvalidOperationException("SubSteward has not been initialized by Emby.");
                }

                return instance;
            }
        }

        public Plugin(IApplicationPaths applicationPaths, IXmlSerializer xmlSerializer)
            : base(applicationPaths, xmlSerializer)
        {
            instance = this;
        }

        public override string Name => PluginIdentity.DisplayName;

        public override Guid Id => PluginIdentity.Id;

        public override string Description => PluginIdentity.Description;

        public IEnumerable<PluginPageInfo> GetPages()
        {
            return new[]
            {
                new PluginPageInfo
                {
                    Name = "SubStewardUI8.html",
                    DisplayName = PluginIdentity.DisplayName,
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.configPage.html",
                    EnableInMainMenu = true,
                    IsMainConfigPage = true,
                    MenuIcon = "subtitles"
                },
                new PluginPageInfo
                {
                    Name = "SubStewardUI8.js",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.subSteward.js"
                },
                // Keep already-open UI3-UI7 URLs functional while Emby/browser caches expire.
                new PluginPageInfo
                {
                    Name = "SubStewardUI7.html",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.configPage.html"
                },
                new PluginPageInfo
                {
                    Name = "SubStewardUI7.js",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.subSteward.js"
                },
                new PluginPageInfo
                {
                    Name = "SubStewardUI3.html",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.configPage.html"
                },
                new PluginPageInfo
                {
                    Name = "SubStewardUI3.js",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.subSteward.js"
                },
                new PluginPageInfo
                {
                    Name = "SubStewardUI4.html",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.configPage.html"
                },
                new PluginPageInfo
                {
                    Name = "SubStewardUI4.js",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.subSteward.js"
                },
                new PluginPageInfo
                {
                    Name = "SubStewardUI5.html",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.configPage.html"
                },
                new PluginPageInfo
                {
                    Name = "SubStewardUI5.js",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.subSteward.js"
                },
                new PluginPageInfo
                {
                    Name = "SubStewardUI6.html",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.configPage.html"
                },
                new PluginPageInfo
                {
                    Name = "SubStewardUI6.js",
                    EmbeddedResourcePath = "SubSteward.Plugin.Web.subSteward.js"
                }
            };
        }
    }
}
