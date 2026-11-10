'use strict';
// Original Reflect Go filtering. This searches the bundled catalog only.
const ReflectGoCatalog = {
  estimate: model => model.estimated_memory_2k || model.size * 1.3 + 1073741824 + 2048 * 524288,
  filter(models, options) {
    const query = (options.query || '').trim().toLowerCase();
    const installed = new Set(options.installed || []);
    return models.filter(model => {
      if (query && ![model.name, model.developer, model.filename, model.quantization].join(' ').toLowerCase().includes(query)) return false;
      if (options.developer && model.developer !== options.developer) return false;
      if (options.purpose !== 'all' && options.purpose !== model.purpose) return false;
      if (options.status === 'installed' && !installed.has(model.filename)) return false;
      if (options.status === 'download' && installed.has(model.filename)) return false;
      if (model.size > options.maxBytes) return false;
      if (options.fitOnly && (!options.budget || ReflectGoCatalog.estimate(model) > options.budget)) return false;
      return true;
    }).sort((a, b) => a.size - b.size || a.name.localeCompare(b.name));
  }
};
if (typeof module !== 'undefined') module.exports = ReflectGoCatalog;
