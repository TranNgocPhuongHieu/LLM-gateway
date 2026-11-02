/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/privacy-policy")({
  component: () => (
    <main className="mx-auto max-w-2xl space-y-5 px-6 py-12">
      <h1 className="text-2xl font-bold">Chính sách quyền riêng tư</h1>
      <p className="text-muted-foreground">
        Trang web sử dụng Cloudflare Turnstile để chống bot và bảo vệ quá trình xác thực.
      </p>
      <a
        className="text-primary underline underline-offset-4"
        href="https://www.cloudflare.com/cloudflare-customer-privacy-addendum/"
        target="_blank"
        rel="noopener noreferrer"
      >
        Cloudflare Turnstile Privacy Addendum
      </a>
    </main>
  ),
});
