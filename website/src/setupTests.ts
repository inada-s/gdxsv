// jest-dom adds custom jest matchers for asserting on DOM nodes.
// allows you to do things like:
// expect(element).toHaveTextContent(/react/i)
// learn more: https://github.com/testing-library/jest-dom
import '@testing-library/jest-dom/vitest';

// gtag.js is loaded via a <script> tag in index.html in the real app; jsdom
// doesn't execute it, so stub it out for components that call window.gtag.
window.gtag = window.gtag || (() => {});
