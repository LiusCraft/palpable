#include "wifi_board.h"
#include "codecs/no_audio_codec.h"
#include "display/lcd_display.h"
#include "application.h"
#include "button.h"
#include "config.h"
#include "assets/lang_config.h"

#include <esp_log.h>
#include <esp_sntp.h>
#include <driver/i2c_master.h>
#include <driver/spi_master.h>
#include <esp_lcd_panel_io.h>
#include <esp_lcd_panel_ops.h>
#include <esp_lcd_panel_vendor.h>
#include <sys/time.h>
#include <time.h>

#define TAG "WghC2Board"

class WghC2Board : public WifiBoard {
private:
    i2c_master_bus_handle_t i2c_bus_ = nullptr;
    Display* display_ = nullptr;
    Button pwr_button_;
    Button volume_up_button_;
    Button volume_down_button_;

    void InitializeI2c() {
        i2c_master_bus_config_t bus_config = {
            .i2c_port = (i2c_port_t)0,
            .sda_io_num = AUDIO_CODEC_I2C_SDA_PIN,
            .scl_io_num = AUDIO_CODEC_I2C_SCL_PIN,
            .clk_source = I2C_CLK_SRC_DEFAULT,
            .glitch_ignore_cnt = 7,
            .intr_priority = 0,
            .trans_queue_depth = 0,
            .flags = {
                .enable_internal_pullup = 1,
            },
        };
        ESP_ERROR_CHECK(i2c_new_master_bus(&bus_config, &i2c_bus_));

        // 先扫一遍总线，确认 ES8311 是否真的在 0x18，避免 codec 初始化失败后反复重启。
        for (int addr = 1; addr < 127; addr++) {
            if (i2c_master_probe(i2c_bus_, addr, 50) == ESP_OK) {
                ESP_LOGI(TAG, "I2C device found at 0x%02x", addr);
            }
        }
    }

    void InitializeSpi() {
        ESP_LOGI(TAG, "Initialize SPI bus for ST7789");
        spi_bus_config_t bus_config = {};
        bus_config.mosi_io_num = DISPLAY_MOSI_PIN;
        bus_config.miso_io_num = DISPLAY_MISO_PIN;
        bus_config.sclk_io_num = DISPLAY_CLK_PIN;
        bus_config.quadwp_io_num = GPIO_NUM_NC;
        bus_config.quadhd_io_num = GPIO_NUM_NC;
        bus_config.max_transfer_sz = DISPLAY_WIDTH * DISPLAY_HEIGHT * sizeof(uint16_t);
        ESP_ERROR_CHECK(spi_bus_initialize(SPI3_HOST, &bus_config, SPI_DMA_CH_AUTO));
    }

    void InitializeLcdDisplay() {
        esp_lcd_panel_io_handle_t panel_io = nullptr;
        esp_lcd_panel_handle_t panel = nullptr;

        ESP_LOGI(TAG, "Install ST7789 panel IO");
        esp_lcd_panel_io_spi_config_t io_config = {};
        io_config.cs_gpio_num = DISPLAY_CS_PIN;
        io_config.dc_gpio_num = DISPLAY_DC_PIN;
        io_config.spi_mode = DISPLAY_SPI_MODE;
        io_config.pclk_hz = 40 * 1000 * 1000;
        io_config.trans_queue_depth = 10;
        io_config.lcd_cmd_bits = 8;
        io_config.lcd_param_bits = 8;
        ESP_ERROR_CHECK(esp_lcd_new_panel_io_spi(SPI3_HOST, &io_config, &panel_io));

        ESP_LOGI(TAG, "Install ST7789 driver");
        esp_lcd_panel_dev_config_t panel_config = {};
        panel_config.reset_gpio_num = DISPLAY_RST_PIN;
        panel_config.rgb_ele_order = DISPLAY_RGB_ORDER;
        panel_config.bits_per_pixel = 16;
        ESP_ERROR_CHECK(esp_lcd_new_panel_st7789(panel_io, &panel_config, &panel));

        esp_err_t err = esp_lcd_panel_reset(panel);
        if (err != ESP_OK) {
            ESP_LOGW(TAG, "LCD reset failed: %s", esp_err_to_name(err));
        }
        err = esp_lcd_panel_init(panel);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "LCD init failed: %s", esp_err_to_name(err));
            display_ = new NoDisplay();
            return;
        }
        esp_lcd_panel_invert_color(panel, DISPLAY_INVERT_COLOR);
        esp_lcd_panel_swap_xy(panel, DISPLAY_SWAP_XY);
        esp_lcd_panel_mirror(panel, DISPLAY_MIRROR_X, DISPLAY_MIRROR_Y);

        display_ = new SpiLcdDisplay(panel_io, panel, DISPLAY_WIDTH, DISPLAY_HEIGHT,
                                     DISPLAY_OFFSET_X, DISPLAY_OFFSET_Y, DISPLAY_MIRROR_X,
                                     DISPLAY_MIRROR_Y, DISPLAY_SWAP_XY);
    }

    void InitializeButtons() {
        pwr_button_.OnClick([this]() {
            Application::GetInstance().ToggleChatState();
        });
        pwr_button_.OnLongPress([this]() {
            EnterWifiConfigMode();
        });
        pwr_button_.OnMultipleClick([this]() {
            EnterWifiConfigMode();
        }, 3);

        volume_up_button_.OnClick([this]() {
            auto codec = GetAudioCodec();
            auto volume = codec->output_volume() + 10;
            if (volume > 100) {
                volume = 100;
            }
            codec->SetOutputVolume(volume);
            GetDisplay()->ShowNotification(Lang::Strings::VOLUME + std::to_string(volume / 10));
        });

        volume_down_button_.OnClick([this]() {
            auto codec = GetAudioCodec();
            auto volume = codec->output_volume() - 10;
            if (volume < 0) {
                volume = 0;
            }
            codec->SetOutputVolume(volume);
            GetDisplay()->ShowNotification(Lang::Strings::VOLUME + std::to_string(volume / 10));
        });
    }

public:
    WghC2Board()
        : pwr_button_(PWR_BUTTON_GPIO),
          volume_up_button_(VOLUME_UP_BUTTON_GPIO),
          volume_down_button_(VOLUME_DOWN_BUTTON_GPIO) {
        // wss 证书校验需要可信时间；先用编译时间兜底，WiFi 起来后再由 SNTP 校正。
        struct tm build_tm = {};
        if (strptime(__DATE__ " " __TIME__, "%b %d %Y %H:%M:%S", &build_tm) != nullptr) {
            time_t build_time = mktime(&build_tm);
            if (build_time > 1700000000) {
                struct timeval tv = {.tv_sec = build_time, .tv_usec = 0};
                settimeofday(&tv, nullptr);
            }
        }

        InitializeI2c();
        InitializeSpi();
        InitializeLcdDisplay();
        InitializeButtons();
        GetBacklight()->RestoreBrightness();
    }

    virtual void StartNetwork() override {
        WifiBoard::StartNetwork();
        esp_sntp_setoperatingmode(ESP_SNTP_OPMODE_POLL);
        esp_sntp_setservername(0, "pool.ntp.org");
        esp_sntp_init();
    }

    virtual AudioCodec* GetAudioCodec() override {
        // 屏幕阶段先用无 codec 的 I2S 通道，避免 ES8311 不在线时整机重启；
        // 确认 I2C 扫描结果后再切回 Es8311AudioCodec。
        static NoAudioCodecDuplex audio_codec(
            AUDIO_INPUT_SAMPLE_RATE, AUDIO_OUTPUT_SAMPLE_RATE,
            AUDIO_I2S_GPIO_BCLK, AUDIO_I2S_GPIO_WS,
            AUDIO_I2S_GPIO_DOUT, AUDIO_I2S_GPIO_DIN);
        return &audio_codec;
    }

    virtual Display* GetDisplay() override { return display_; }

    virtual Backlight* GetBacklight() override {
        static PwmBacklight backlight(DISPLAY_BACKLIGHT_PIN, DISPLAY_BACKLIGHT_OUTPUT_INVERT);
        return &backlight;
    }
};

DECLARE_BOARD(WghC2Board);
