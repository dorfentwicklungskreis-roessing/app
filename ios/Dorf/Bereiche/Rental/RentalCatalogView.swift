import SwiftUI

/// „Maschinchenring" — the devices the village lends to the village.
///
/// The catalogue is public: it is here before anybody signs in, and it stays
/// here when the sign-in has gone stale. Booking is the part that needs the
/// Rössing-ID, and it says so where it is needed, not in front of the door.
struct RentalCatalogView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @State private var model: RentalCatalogModel?

    var body: some View {
        Group {
            if let model {
                RentalCatalogList(model: model)
            } else {
                ProgressView().controlSize(.large)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle("Maschinchenring")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                NavigationLink {
                    RentalBookingsView()
                } label: {
                    Image(systemName: "calendar.badge.clock")
                }
                .accessibilityLabel("Meine Buchungen")
                .accessibilityIdentifier("rental-my-bookings")
            }
        }
        .task {
            let existing = model ?? RentalCatalogModel(source: .from(umgebung.rental))
            model = existing
            await existing.load()
        }
    }
}

/// The list itself, once the model stands.
struct RentalCatalogList: View {
    @ObservedObject var model: RentalCatalogModel

    var body: some View {
        List {
            // The note first, then the (possibly older) list. An empty page
            // without an explanation would be the worst result.
            if let hint = model.hint {
                Section {
                    RentalNotice(text: hint) {
                        Task { await model.refresh() }
                    }
                }
            }

            Section {
                if model.loading && model.devices.isEmpty {
                    HStack(spacing: 10) {
                        ProgressView()
                        Text("Geräte werden geholt …").foregroundStyle(.secondary)
                    }
                    .accessibilityIdentifier("rental-loading")
                }

                ForEach(model.visible) { device in
                    NavigationLink {
                        RentalDeviceView(device: device)
                    } label: {
                        RentalDeviceRow(device: device)
                    }
                    .accessibilityIdentifier("rental-device-\(device.id)")
                }

                if model.withoutMatch {
                    Text("Dazu gibt es kein Gerät. Versuch es mit einem anderen Wort.")
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("rental-no-match")
                }

                if model.empty {
                    Text("Im Maschinchenring steht gerade nichts zum Ausleihen. "
                        + "Sobald jemand ein Gerät einstellt, steht es hier.")
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("rental-empty")
                }
            } footer: {
                Text("Die Geräte gehören Leuten aus dem Dorf und werden im "
                    + "Maschinchenring gepflegt — hier stehen sie nur.")
            }
        }
        .accessibilityIdentifier("rental-catalog")
        .searchable(text: $model.query, prompt: "Gerät suchen")
        .refreshable { await model.refresh() }
    }
}

/// One row of the catalogue.
struct RentalDeviceRow: View {
    let device: RentalDevice

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            RentalThumbnail(url: device.imageURL)

            VStack(alignment: .leading, spacing: 3) {
                Text(device.name)
                    .font(.headline)
                    .fixedSize(horizontal: false, vertical: true)

                if !device.summary.isEmpty {
                    Text(device.summary)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }

                HStack(spacing: 8) {
                    if let price = device.priceText {
                        Text(price)
                            .font(.footnote.weight(.medium))
                            .foregroundStyle(.tint)
                    }
                    if let category = device.category {
                        Text(category)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }

                // Greyed out because the platform said so — never because
                // this app worked something out on its own.
                if !device.bookable {
                    Text(device.unavailableNote ?? "Gerade nicht buchbar.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
    }
}

/// The little picture in front of a row. A device without one keeps its place
/// in the column, so the names stay aligned.
struct RentalThumbnail: View {
    let url: URL?

    var body: some View {
        Group {
            if let url {
                AsyncImage(url: url) { phase in
                    switch phase {
                    case .success(let bild):
                        bild.resizable().scaledToFill()
                    default:
                        placeholder
                    }
                }
            } else {
                placeholder
            }
        }
        .frame(width: 56, height: 56)
        .clipShape(RoundedRectangle(cornerRadius: 10))
        .accessibilityHidden(true)
    }

    private var placeholder: some View {
        ZStack {
            Color.accentColor.opacity(0.10)
            Image(systemName: "wrench.and.screwdriver")
                .font(.title3)
                .foregroundStyle(.tint)
        }
    }
}

/// The note above a list — with the way back, not just the bad news.
struct RentalNotice: View {
    let text: String
    var actionTitle: String = "Erneut versuchen"
    let action: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(text, systemImage: "exclamationmark.triangle.fill")
                .font(.subheadline)
            Button(actionTitle, action: action)
                .buttonStyle(.borderless)
                .accessibilityIdentifier("rental-notice-action")
        }
        .padding(.vertical, 4)
        .accessibilityIdentifier("rental-notice")
    }
}
