using System;
using MediaBrowser.Common.Configuration;
using MediaBrowser.Common.Plugins;
using MediaBrowser.Model.Serialization;

namespace SubSteward.Plugin
{
    /// <summary>
    /// The stable Emby plugin identity and its server-managed data folder.
    /// </summary>
    public sealed class Plugin : BasePlugin<PluginConfiguration>
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
    }
}
