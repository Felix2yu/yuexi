/** @type {import('tailwindcss').Config} */
// Local Tailwind build config. Used to generate internal/handler/static/tailwind.css
// from the server-rendered templates, replacing the runtime Tailwind Play CDN.
// Regenerate with:
//   npx tailwindcss -c tailwind.config.js -i tailwind.in.css -o internal/handler/static/tailwind.css --minify
module.exports = {
  content: [
    './internal/handler/template/**/*.html',
    './internal/handler/static/sw.js',
  ],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        sans: [
          'system-ui', '-apple-system', 'BlinkMacSystemFont',
          '"PingFang SC"', '"Microsoft YaHei"', '"Segoe UI"',
          'Roboto', 'Helvetica', 'Arial', 'sans-serif',
        ],
        serif: [
          'ui-serif', 'Georgia', '"Songti SC"', '"SimSun"',
          '"Noto Serif CJK SC"', 'serif',
        ],
        display: [
          'ui-serif', 'Georgia', '"Songti SC"', '"SimSun"',
          '"Noto Serif CJK SC"', 'serif',
        ],
      },
      colors: {
        rose: {
          50: '#fdf2f8', 100: '#fce7f3', 200: '#fbcfe8', 300: '#f9a8d4',
          400: '#f472b6', 500: '#ec4899', 600: '#db2777',
        },
        mauve: {
          50: '#faf5ff', 100: '#f3e8ff', 200: '#e9d5ff', 300: '#d8b4fe',
          400: '#c084fc', 500: '#a855f7',
        },
        cream: {
          50: '#fefdfb', 100: '#fdf8f3', 200: '#f9f0e6',
        },
        ink: {
          900: '#1a1625', 800: '#2d2640', 700: '#3d3555', 600: '#4e4568',
        },
      },
    },
  },
  plugins: [],
}
