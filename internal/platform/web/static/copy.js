/** @fileoverview Shows each Copy button, which needs a script, and copies the text of the element its data-copy names,
 * saying Copied for a moment. Without JavaScript the buttons stay hidden, and one click still selects the whole text,
 * which the page's styles allow, to copy by hand. */
(() => {
  document.addEventListener('DOMContentLoaded', () => {
    for (const button of document.querySelectorAll('button[data-copy]')) {
      const target = document.getElementById(button.dataset.copy);
      const label = button.querySelector('[data-copy-label]');
      if (!target || !label || !navigator.clipboard) continue;
      const text = label.textContent;
      let reset;
      button.hidden = false;
      button.addEventListener('click', async () => {
        try {
          await navigator.clipboard.writeText(target.textContent);
          label.textContent = 'Copied';
        } catch {
          // The browser refused, such as without permission: select the text, to copy by hand.
          window.getSelection().selectAllChildren(target);
          label.textContent = 'Select and copy';
        }
        clearTimeout(reset);
        reset = setTimeout(() => { label.textContent = text; }, 2000);
      });
    }
  });
})();
