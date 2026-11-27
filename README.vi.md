<p align="center">
  <a href="README.md">English</a> · <strong><a href="README.vi.md">Tiếng Việt</a></strong>
</p>

<p align="center"><strong>Modex</strong></p>
<h1 align="center">Dùng agent lập trình trên điện thoại.</h1>
<p align="center">Ứng dụng Android native để làm việc với agent trên máy tính của bạn qua SSH.</p>

<p align="center">
  <a href="https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.38"><img alt="Bản xem trước mới nhất" src="https://img.shields.io/badge/preview-v0.1.0--dev.38-blue"></a>
  <a href="https://github.com/warmdev17/modex/actions/workflows/verify.yml"><img alt="CI" src="https://github.com/warmdev17/modex/actions/workflows/verify.yml/badge.svg"></a>
</p>

> **Phần mềm xem trước.** Modex đang được phát triển và chưa sẵn sàng cho môi trường production. Các bản hiện tại là bản phát triển.

## Làm việc với agent trên máy tính

Modex cung cấp giao diện chat Android native cho các runtime agent được cài trên máy Linux hoặc macOS của bạn. Điện thoại kết nối qua SSH; máy host chạy agent với tài khoản và cấu hình sẵn có. Hiện tại, muốn dùng Windows làm host thì cần WSL2.

- Bắt đầu và tiếp tục hội thoại agent trên Android.
- Theo dõi phản hồi trực tiếp và xem các yêu cầu quyền được hỗ trợ.
- Giữ thông tin xác thực provider và quá trình chạy trên host.
- Dùng Codex, OpenCode hoặc Antigravity tùy cấu hình host và khả năng được hỗ trợ.

## Tải xuống

Bản xem trước hiện tại là [v0.1.0-dev.38](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.38). Tải APK Android từ GitHub pre-release. Package ứng dụng là `dev.modex`, yêu cầu Android 8.0 (API 26) trở lên. Xem [hướng dẫn cài đặt](docs/install.md) để biết cách cài host và thông tin nền tảng.

## Bắt đầu nhanh

Bản pre-release dev.38 phát hành APK Android. Sau khi cài Go 1.23 trở lên, hãy build bộ công cụ host tương ứng từ checkout này trên Linux hoặc macOS:

```sh
./scripts/build.sh
~/.local/bin/modex service install --workspace /absolute/path/to/project
~/.local/bin/modex service start
```

Khi cần, build script sẽ thêm `~/.local/bin` vào file khởi động của các shell được hỗ trợ; hãy mở terminal mới trước khi dùng lệnh `modex` ngắn hơn.

Cấu hình đăng nhập provider cho agent và SSH public key trên host. Trong Android, thêm hostname/IP, port, user SSH, cùng đường dẫn binary/state mà `modex service install` in ra. Import một private key riêng, **không mã hóa**, đã được cấp quyền cho user SSH đó.

Paste fingerprint SHA256 của host key SSH sau khi kiểm tra trực tiếp trên host hoặc qua một kênh tin cậy khác. Ví dụ với host Linux dùng key Ed25519:

```sh
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256
```

Không tin fingerprint chỉ lấy từ một lần quét mạng chưa xác minh. Xem [quản lý kết nối Android](docs/mobile-development.md#managing-android-connections) và [hướng dẫn cài đặt](docs/install.md) để biết toàn bộ quy trình và ghi chú WSL2.

## Cách hoạt động

```mermaid
flowchart LR
  A[Ứng dụng Modex Android] -->|SSH| B[modex-host supervisor và journal]
  B --> C[Adapter agent và runtime]
  C --> D[Provider]
```

Kết nối SSH truyền yêu cầu từ ứng dụng và sự kiện agent. Việc chạy agent và thông tin xác thực provider nằm trên host. SSH bảo vệ đường truyền; Modex không tuyên bố mã hóa đầu cuối vượt ngoài SSH.

## Hỗ trợ runtime

| Runtime | Phạm vi hiện tại |
| --- | --- |
| Codex | Tích hợp đầy đủ nhất, gồm hội thoại, streaming, các phê duyệt được hỗ trợ, lịch sử, tệp đính kèm và một số điều khiển riêng của Codex. |
| OpenCode | Chat, lịch sử, chọn model, tệp đính kèm, project, form và trạng thái công cụ có mức hỗ trợ khác nhau. Một số điều khiển riêng Codex và tính năng OpenCode chưa hỗ trợ; xem [ma trận hỗ trợ agent hiện tại](docs/vi/agents.md). |
| Antigravity | Chat văn bản, model mặc định, streaming, Stop, lịch sử và quyền dùng một lần. Chưa hỗ trợ tệp đính kèm và điều hướng prompt; không đảm bảo tương đương tính năng. |

Các thao tác host được hỗ trợ trên Linux và macOS. Quy trình host trên Windows hiện dùng WSL2. Xem [chi tiết hỗ trợ agent](docs/vi/agents.md).

## Các dự án tương tự

Đây là danh sách chọn lọc, không phải toàn bộ thị trường. Mức tương thích bên dưới là nội dung từng dự án công bố tại ngày **2026-11-27**, không phải kết quả kiểm thử tương tác của Modex. Hỗ trợ được nêu tên có thể dùng tích hợp riêng, ACP hoặc terminal nên độ sâu tính năng sẽ khác nhau giữa các agent.

| Dự án | Điểm nổi bật | Mức tương thích agent được công bố |
| --- | --- | --- |
| [T3 Code](https://github.com/pingdotgg/t3code) | Control plane trên desktop, web và mobile; worktree, review Git, truy cập từ xa và giao việc chéo agent. Xem ghi chú telemetry bên dưới. | Công bố hỗ trợ Claude Code, Codex, Cursor, OpenCode, Grok Build, Antigravity, Kiro, Pi, Muse Code và fx; đồng thời nhận agent ACP local hoặc từ registry. |
| [Happy](https://github.com/slopus/happy) | Client mobile/web/desktop, relay mã hóa, push notification, phiên nhiều người và orchestration. | Đường tích hợp trực tiếp cho Claude, Codex và Grok; Gemini qua Antigravity; OpenCode cùng agent ACP khác qua harness. |
| [Sesori](https://github.com/sesori-ai/sesori_apps_monorepo) | Điều khiển từ xa trên iOS/Android, relay mã hóa, diff, worktree, xem subagent, notification và cài runtime được quản lý. | Claude Code, Codex, OpenCode, Cursor, GitHub Copilot, Antigravity, Grok Build, DeepSeek Harness, Hermes Agent, Pi và Oh My Pi. |
| [OpenCode Mobile](https://github.com/alvarolorentedev/opencode-mobile) | Companion iOS/Android tập trung cho mobile, có tool trực tiếp, approval, diff, terminal, voice và kết nối thẳng tới server tự host. | Chỉ OpenCode; dùng model, provider, agent, command và plugin mà server OpenCode đang kết nối cung cấp. |
| [AndCode](https://github.com/yuga-hashimoto/and-code) | Chạy agent ngay trên Android qua PRoot nên không cần PC hay SSH khi dùng on-device; đồng thời hỗ trợ OpenCode từ xa. | OpenCode stable; Claude Code, Codex và Antigravity được đánh dấu beta. Chế độ remote PC hiện được ghi rõ cho OpenCode. |
| [MobileFlow](https://github.com/cwx2/mobile-flow) | Client điện thoại và desktop agent với file, terminal, Git, local preview, relay và tunnel tự host. | Đường ACP cho Claude Code, Codex, Aider, Gemini CLI, GitHub Copilot và Kiro; công cụ CLI khác chạy qua terminal. |

**Ghi chú telemetry của T3 Code.** Một [issue cộng đồng](https://github.com/pingdotgg/t3code/issues/4123) phản đối telemetry bật mặc định và thông điệp quyền riêng tư trước đó; issue này được đóng ngày 4/11/2026. [Tài liệu telemetry hiện tại](https://github.com/pingdotgg/t3code/blob/main/docs/user/telemetry.md) của T3 Code cho biết sản phẩm gửi sự kiện sử dụng và chẩn đoán relay, cung cấp biến môi trường để opt-out trên server/desktop cùng các toggle chẩn đoán trên mobile, đồng thời nói prompt, response, nội dung file và credential không được gửi trừ khi người dùng chủ động duyệt kèm nội dung hội thoại trong báo cáo agent-feedback. Xem thêm [bài KuCoin](https://www.kucoin.com/vi/news/flash/t3-code-telemetry-controversy-sparks-community-outcry-and-code-fixes). Hãy tự kiểm tra policy và setting hiện tại trước khi dùng bất kỳ công cụ remote agent nào với repository nhạy cảm.

Modex hiện chọn phạm vi hẹp hơn: Android native qua SSH do người dùng quản lý, không bắt buộc tài khoản hay relay Modex, credential nằm trên máy đích và từng runtime có ranh giới khả năng rõ ràng. Modex chưa có độ phủ Claude Code, truy cập remote được host sẵn, push notification hay tổng số agent rộng bằng các dự án trên.

## Tài liệu

- [Kiến trúc](docs/vi/architecture.md) · [Hỗ trợ agent](docs/vi/agents.md)
- [Trạng thái hiện tại](docs/vi/status.md) · [Lộ trình](docs/vi/roadmap.md)
- [Mục lục tài liệu](docs/vi/README.md)
- [Cài đặt](docs/install.md) · [Ghi chú thiết kế](docs/design.md) · [Quy trình phát hành](docs/releasing.md)

## Định hướng tương lai

Modex Cloud, relay Basic/Plus, thực thi do Modex quản lý ở gói Pro, thông báo đẩy và điều phối nhiều agent đều là định hướng tương lai, chưa có trong bản xem trước này. Giá, thanh toán, cách xử lý tài khoản provider và các chi tiết thương mại liên quan vẫn chưa được quyết định.

Modex chưa có giấy phép dự án. Xem [trạng thái hiện tại](docs/vi/status.md) để biết phần nào đã triển khai và phần nào vẫn đang xem trước.
