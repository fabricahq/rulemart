/** @fileoverview Closes the page's menus, each a <details data-menu>, as a menu should: when a click or focus leaves
 * it, and on Escape, which returns focus to its button. Without JavaScript, each still opens and closes with its
 * button. Also focuses the header's search field when / is pressed while nothing else has focus. */
(() => {
  document.addEventListener('keydown', (event) => {
    if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey) return;
    if (document.activeElement && document.activeElement !== document.body) return;
    const search = document.querySelector('#header-search');
    if (!search || !search.offsetParent) return;
    event.preventDefault();
    search.focus();
  });
  document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('details[data-menu]').forEach((menu) => {
      const trigger = menu.querySelector('summary');
      document.addEventListener('click', (event) => {
        if (!menu.contains(event.target)) menu.open = false;
      });
      document.addEventListener('keydown', (event) => {
        if (event.key === 'Escape' && menu.open) {
          event.preventDefault();
          menu.open = false;
          trigger.focus();
        }
      });
      menu.addEventListener('focusout', (event) => {
        if (event.relatedTarget && !menu.contains(event.relatedTarget)) menu.open = false;
      });
    });
  });
})();
