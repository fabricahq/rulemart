/** @fileoverview Shows each Copy button, which needs a script, and copies the text of the element its data-copy names,
 * saying Copied for a moment, and to screen readers through the status beside it. Without JavaScript the buttons stay
 * hidden, and one click still selects a whole block, which the page's styles allow, to copy by hand. Select all, with
 * a block focused, selects only the block. */
(() => {
  document.addEventListener('DOMContentLoaded', () => {
    for (const block of document.querySelectorAll('[data-select-all]')) {
      block.addEventListener('keydown', (event) => {
        if ((event.metaKey || event.ctrlKey) && !event.altKey && event.key.toLowerCase() === 'a') {
          event.preventDefault();
          window.getSelection().selectAllChildren(block);
        }
      });
    }
    for (const button of document.querySelectorAll('button[data-copy]')) {
      const target = document.getElementById(button.dataset.copy);
      const label = button.querySelector('[data-copy-label]');
      const status = document.querySelector(`[data-copy-status="${button.dataset.copy}"]`);
      if (!target || !label || !navigator.clipboard) continue;
      const text = label.textContent;
      let reset;
      button.hidden = false;
      button.addEventListener('click', async () => {
        let said;
        try {
          await navigator.clipboard.writeText(target.textContent);
          said = 'Copied';
        } catch {
          // The browser refused, such as without permission: select the text, to copy by hand.
          window.getSelection().selectAllChildren(target);
          said = 'Selected; copy it by hand';
        }
        label.textContent = said === 'Copied' ? 'Copied' : 'Select and copy';
        if (status) status.textContent = said;
        clearTimeout(reset);
        reset = setTimeout(() => {
          label.textContent = text;
          if (status) status.textContent = '';
        }, 2000);
      });
    }
  });
})();
