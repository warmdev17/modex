# Hỗ trợ agent

[English](../agents.md) · **Tiếng Việt**

Phạm vi hỗ trợ khác nhau theo từng runtime. Modex không tuyên bố các agent có bộ tính năng tương đương. Hãy cài và đăng nhập runtime trên host trước khi dùng; bridge đang chạy không chứng minh provider đã xác thực hay inference hoạt động.

## Kết nối agent

1. Cài runtime trên host và đăng nhập provider theo quy trình bình thường của runtime đó.
2. Cài Modex rồi khởi động host service cho workspace. Service mặc định dùng Codex; OpenCode cần cấu hình backend riêng theo [hướng dẫn backend](../opencode-backend.md).
3. Bật đăng nhập SSH bằng public key, kiểm tra fingerprint của host key bên ngoài Modex, sau đó thêm host vào ứng dụng Android.
4. Mở hoặc tạo project rồi chọn runtime khi tạo chat mới. Antigravity chỉ xuất hiện trên kết nối dùng Codex sau khi host xác nhận CLI, đăng nhập và khả năng khôi phục phiên ACP đã sẵn sàng.

Chat hiện có tiếp tục dùng runtime và phiên native đã chọn. Kết nối host không khóa toàn bộ project hay chat vào một agent.

| Runtime | Có trong bản xem trước này | Giới hạn đã biết |
| --- | --- | --- |
| Codex | Đường tích hợp đầy đủ nhất: chat, streaming, lịch sử, tệp đính kèm, phê duyệt/câu hỏi được hỗ trợ và các điều khiển riêng Codex mà host công bố. | Một số khả năng và thao tác native của Codex chưa có; xem [tài liệu protocol](../history-contract.md) và [thiết kế](../design.md). |
| OpenCode | Chat, lịch sử, chọn model, tệp đính kèm, ngữ cảnh project, form và trạng thái công cụ. | Chưa có plan mode, permission profile, takeover, reference hay toàn bộ điều khiển project riêng Codex. Trạng thái phiên chỉ tồn tại theo supervisor. |
| Antigravity | Prompt văn bản, streaming, Stop, lịch sử, quyền dùng một lần và chọn model, mức suy nghĩ trên host tương thích. | Đính kèm và gửi thêm prompt cần bản phát triển mô tả bên dưới. |

Chỉ có executable không đồng nghĩa agent đã sẵn sàng. Hiện tại, bộ kiểm tra Antigravity xác nhận CLI, đăng nhập và các khả năng ACP bắt buộc trước khi cho phép chọn. Host Codex và OpenCode khởi động thành công chỉ chứng minh bridge đã khởi tạo, không chứng minh provider đã xác thực hay inference sẽ thành công. Tài khoản provider và thông tin xác thực được cấu hình, kiểm tra trên host.

Ma trận này mô tả bản xem trước hiện tại. [Snapshot triển khai backend OpenCode](../opencode-backend.md) ghi lại một revision đã kiểm chứng trước đây và có thể chưa gồm phần project, form hay lịch sử tool mới hơn.

## Chat nhiều agent trong bản phát triển

Khi đã cập nhật cả host và Android, kết nối SSH một lần rồi chọn Codex,
OpenCode hoặc Antigravity trong **New chat**. Có thể lọc danh sách theo
agent. Các chat cùng project giữ agent và lịch sử riêng.

Đường OpenCode do Modex quản lý cần OpenCode từ 2.4 trong dòng 2.x. Nó
dùng cơ sở dữ liệu riêng trong thư mục trạng thái supervisor. Chat và
tài khoản OpenCode độc lập vẫn nằm trong hồ sơ hiện có. Hãy cấu hình
quyền truy cập provider trên host và kiểm tra model cần dùng trong hồ sơ
được quản lý trước khi làm việc.

Để đăng nhập OpenCode tương tác, đặt `supervisor_state` thành thư mục
trạng thái đã cấu hình trong Modex. Chạy bằng chính tài khoản người dùng
trên host, sau khi lần kiểm tra runtime đầu tiên đã tạo hồ sơ.

```sh
profile="$supervisor_state/agent-chats/opencode"
XDG_DATA_HOME="$profile/data" XDG_STATE_HOME="$profile/state" \
  OPENCODE_DB="$profile/data/opencode/opencode.db" opencode auth login --standalone
```

Làm mới danh sách agent sau khi đăng nhập.

## Antigravity trong bản phát triển

Khi cả host và Android hỗ trợ tùy chọn phiên Antigravity, bộ chọn trong
chat hiển thị các model và mức suy nghĩ do ACP cung cấp. Có thể đổi khi chat
đang rảnh. Modex lưu lựa chọn đã được xác nhận theo từng chat và đối chiếu
với các tùy chọn còn khả dụng khi mở lại. Host cũ và phiên ACP thiếu tùy
chọn tiếp tục dùng model mặc định của Antigravity.
Antigravity 1.5.17 đưa chat mới vào danh sách khôi phục sau lượt chat đầu
tiên được ghi lại.

Trên bản tương thích, đầu chat và Session status hiển thị dung lượng
context do Antigravity báo về. Số token cập nhật theo dữ liệu từ CLI và
có thể cần đợi mẫu mới sau khi mở lại phiên native.

Khi cả host lẫn Android đã cập nhật, tên do Antigravity tạo sẽ hiện ở
đầu chat và trong sidebar, kể cả sau khi kết nối lại. Chat cũ lấy lại
tên khi mở nếu Antigravity cung cấp tên đó.

Khi cả host lẫn Android đã cập nhật, dùng **Attach files** trong chat
Antigravity để gửi ảnh PNG/JPEG hoặc file văn bản UTF-8. Có thể gửi file
riêng hoặc kèm lời nhắn. Mỗi tin nhắn gửi tối đa tám file, tổng 8 MiB.
Mỗi file văn bản tối đa 256 KiB. Khả năng nhận ảnh còn phụ thuộc model
đang dùng. Sau khi host khởi động lại, Modex khôi phục phần xem trước từ
các đường dẫn file mà Antigravity trả về trong lịch sử. Một số file đã gửi
có thể không xuất hiện lại.

Với bản Modex host và Android tương thích cùng Antigravity CLI 1.5.17,
bấm Send lúc agent đang trả lời để gửi thêm prompt văn bản vào cùng
phiên. Đợi phiên rảnh để gửi nháp có file đính kèm. Stop dừng lượt đang
chạy nếu còn prompt chưa hoàn tất.

Bảng Agents hiển thị lượt gọi subagent do Antigravity báo về. Trạng thái
hoàn tất của lượt gọi nghĩa là công cụ đã trả kết quả. Bản CLI được kiểm
thử cung cấp trạng thái lượt gọi, còn lịch sử riêng của subagent chưa có.
